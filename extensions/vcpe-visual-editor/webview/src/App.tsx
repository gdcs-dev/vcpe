import React, { useCallback, useEffect, useRef } from 'react';
import {
  ReactFlow, Background, Controls,
  useNodesState, useEdgesState, reconnectEdge,
  type Node, type Edge, type Connection, type NodeChange, type EdgeChange,
  MarkerType, ConnectionMode, addEdge,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { parse, type ParseResult } from './yaml/parse';
import { applyMutation } from './yaml/serialize';
import { computeInitialLayout } from './layout/autoLayout';
import { useManifestStore } from './store/manifestStore';

import { ServiceNode, type ServiceNodeData, type ServiceItem } from './nodes/ServiceNode';
import { InterfaceEdge } from './edges/InterfaceEdge';
import { DependsOnEdge } from './edges/DependsOnEdge';

import { TypePalette } from './panels/TypePalette';
import { PropertyPanel } from './panels/PropertyPanel';
import { ManifestDropdown } from './panels/ManifestDropdown';
import { WelcomeScreen } from './panels/WelcomeScreen';

import type { ServiceTypeDescriptor, LayoutData, DropTemplate, PaletteVariant } from './types';
import type { Network } from './yaml/parse';

import { vscodeApi as vscode } from './vsCodeApi';

// ─── Default network settings (based on example.yaml) ────────────────────────
// Auto-created when a service is dropped and these roles don't exist yet.
const DEFAULT_NETWORKS: Record<string, Omit<Network, 'role'>> = {
  'mgmt':  { ipv4: { cidr: '10.10.10.0/24',  gateway: '10.10.10.1',  pool: { start: '10.10.10.10',  end: '10.10.10.250'  } } },
  'wan':   { nat: true, firewall: true, ipamDriver: 'none', ipv4: { cidr: '10.7.200.0/24', gateway: '10.7.200.1', pool: { start: '10.7.200.10', end: '10.7.200.250' } } },
  'cm':    { ipamDriver: 'none', ipv4: { cidr: '10.7.201.0/24', gateway: '10.7.201.1', pool: { start: '10.7.201.10', end: '10.7.201.250' } } },
  'lan-p1':{ ipamDriver: 'none' },
  'lan-p2':{ ipamDriver: 'none' },
  'lan-p3':{ ipamDriver: 'none' },
  'lan-p4':{ ipamDriver: 'none' },
};

// Built-in drop templates; applied when no vcpe.serviceDropDefaults entry is set for the type.
const DEFAULT_SERVICE_TEMPLATES: Record<string, DropTemplate> = {
  bng: {
    interfaces: [
      { role: 'mgmt', device: 'mgmt', sharing: 'unique' },
      { role: 'wan',  device: 'wan',  sharing: 'unique' },
      { role: 'cm',   device: 'cm',   sharing: 'unique' },
    ],
  },
  gateway: {
    interfaces: [
      { role: 'wan',    device: 'wan0',    sharing: 'shared' },
      { role: 'cm',     device: 'erouter0', sharing: 'shared' },
      { role: 'lan-p1', device: 'eth0', bridge: 'brlan0', sharing: 'unique' },
      { role: 'lan-p2', device: 'eth1', bridge: 'brlan1', sharing: 'unique' },
    ],
    bridges: [
      { name: 'brlan0', ipv4: '10.0.0.1/24' },
      { name: 'brlan1', ipv4: '10.0.10.1/24' },
    ],
  },
  xb10: {
    interfaces: [
      { role: 'wan',   device: 'wan0', sharing: 'shared' },
      { role: 'cm',    device: 'cm0',  sharing: 'shared' },
      { role: 'lan-p1', device: 'eth0', sharing: 'unique' },
      { role: 'lan-p2', device: 'eth1', sharing: 'unique' },
      { role: 'lan-p3', device: 'eth2', sharing: 'unique' },
      { role: 'lan-p4', device: 'eth3', sharing: 'unique' },
    ],
  },
};

// ─── Custom node/edge registration ───────────────────────────────────────────
const nodeTypes = {
  service: ServiceNode,
};

const edgeTypes = {
  interface: InterfaceEdge,
  dependsOn: DependsOnEdge,
};

// ─── App ──────────────────────────────────────────────────────────────────────
export default function App() {
  const store = useManifestStore();
  const [rfNodes, setRfNodes, onNodesChange] = useNodesState<Node>([]);
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const rawYamlRef = useRef<string>('');
  // layoutRef always holds the latest layout so the stale-closure message
  // handler can read the positions the user has dragged nodes to.
  const layoutRef = useRef<LayoutData | null>(null);
  // edgeHandleOverridesRef persists user-set handle positions (left vs right) across canvas rebuilds.
  const edgeHandleOverridesRef = useRef<Map<string, { sourceHandle: string | null; targetHandle: string | null }>>(new Map());

  // ── Extension message handler ──────────────────────────────────────────────
  useEffect(() => {
    const handler = (event: MessageEvent) => {
      const msg = event.data;
      if (!msg?.type) return;

      switch (msg.type) {
        case 'INIT': {
          rawYamlRef.current = msg.yaml ?? '';
          store.setTypes(msg.types ?? [], msg.typesError ?? null);
          store.setDropDefaults(msg.dropDefaults ?? {});
          store.setPaletteVariants((msg.paletteVariants ?? []).map((v: PaletteVariant) => ({ ...v, _variant: true as const })));
          store.setManifestPath(msg.manifestPath ?? null);
          if (msg.layout) {
            layoutRef.current = msg.layout as LayoutData;
            store.setLayout(msg.layout as LayoutData);
            const savedHandles = (msg.layout as LayoutData).edgeHandles;
            if (savedHandles) {
              edgeHandleOverridesRef.current = new Map(Object.entries(savedHandles));
            }
          }
          updateCanvas(msg.yaml, layoutRef.current);
          break;
        }
        case 'DOCUMENT_UPDATED': {
          rawYamlRef.current = msg.yaml ?? '';
          updateCanvas(msg.yaml, layoutRef.current);
          break;
        }
      }
    };
    window.addEventListener('message', handler);
    vscode?.postMessage({ type: 'READY' });
    return () => window.removeEventListener('message', handler);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ── Canvas builder from ManifestModel ─────────────────────────────────────
  const updateCanvas = useCallback(
    (yamlText: string, existingLayout: LayoutData | null) => {
      if (!yamlText) return;
      const result = parse(yamlText);
      if ('error' in result) {
        store.setYamlError(result.error, result.line);
        setRfNodes([]);
        setRfEdges([]);
        return;
      }

      const { model } = result as ParseResult;
      store.setModel(model, yamlText);

      // Resolve layout: use sidecar if available, else compute with dagre
      let layout = existingLayout;
      let isNewLayout = false;
      if (!layout || Object.keys(layout.nodes).length === 0) {
        layout = computeInitialLayout(model);
        isNewLayout = true;
      }

      const pos = (id: string) => layout?.nodes[id] ?? { x: 0, y: 0 };

      const nodes: Node[] = [];
      const edges: Edge[] = [];

      // ── Service nodes ──────────────────────────────────────────────────────
      for (const svc of model.spec.services) {
        const nodeId = `service:${svc.name}`;

        // Separate interfaces: those with a bridge assignment vs. those without.
        const nonBridgedIfaces = (svc.interfaces ?? []).filter(i => !i.bridge);

        // Build the unified sorted items list (non-bridged interfaces + bridge groups).
        const items: ServiceItem[] = [
          ...nonBridgedIfaces.map(i => ({
            kind: 'interface' as const,
            role: i.role, device: i.device, bridge: i.bridge, ipv4: i.ipv4, defaultRoute: i.defaultRoute,
          })),
          ...(svc.bridges ?? []).map(b => ({
            kind: 'bridge' as const,
            name: b.name,
            ipv4: b.ipv4,
            members: (svc.interfaces ?? [])
              .filter(i => i.bridge === b.name)
              .map(i => ({ role: i.role, device: i.device, ipv4: i.ipv4, defaultRoute: i.defaultRoute }))
              .sort((a, b) => (a.device || a.role).localeCompare(b.device || b.role)),
          })),
        ];
        items.sort((a, b) => {
          const nameA = a.kind === 'bridge' ? a.name : (a.device || a.role);
          const nameB = b.kind === 'bridge' ? b.name : (b.device || b.role);
          return nameA.localeCompare(nameB);
        });

        nodes.push({
          id: nodeId,
          type: 'service',
          position: pos(nodeId),
          data: {
            name: svc.name,
            type: svc.type,
            replicas: svc.replicas,
            items,
          } satisfies ServiceNodeData,
          draggable: true,
        });

        // DependsOn edges (A → B = "A needs B")
        for (const dep of svc.dependsOn ?? []) {
          edges.push({
            id: `dep-${svc.name}-${dep}`,
            source: nodeId,
            target: `service:${dep}`,
            sourceHandle: 'dep-source',
            targetHandle: 'dep-target',
            type: 'dependsOn',
            markerEnd: { type: MarkerType.ArrowClosed, color: '#666' },
          });
        }
      }

      // ── Network edges: one edge per (service-pair, shared-network-role) ─────

      // Build: networkRole → [service names that use it]
      const netServices: Record<string, string[]> = {};
      for (const svc of model.spec.services) {
        for (const iface of svc.interfaces ?? []) {
          if (!netServices[iface.role]) netServices[iface.role] = [];
          if (!netServices[iface.role].includes(svc.name)) {
            netServices[iface.role].push(svc.name);
          }
        }
      }

      // Build a quick type lookup
      const svcType: Record<string, string> = {};
      for (const svc of model.spec.services) svcType[svc.name] = svc.type;

      // Infrastructure service types that "own" networks.
      // rootInfraTypes (bng) connect to all peers; non-root infra (gateway, xb10) connect only to clients.
      const infraTypes = new Set(['bng', 'gateway', 'xb10']);
      const rootInfraTypes = new Set(['bng']);

      for (const [role, svcNames] of Object.entries(netServices)) {
        if (svcNames.length < 2) continue;
        const net = model.spec.networks.find(n => n.role === role);
        const cidr = net?.ipv4?.cidr ?? net?.ipv6?.cidr;
        for (let i = 0; i < svcNames.length; i++) {
          for (let j = i + 1; j < svcNames.length; j++) {
            const [a, b] = [svcNames[i], svcNames[j]].sort();
            const aIsInfra = infraTypes.has(svcType[a]);
            const bIsInfra = infraTypes.has(svcType[b]);
            // Skip client↔client and CPE↔CPE (gateway/xb10 peer) pairs.
            if (!aIsInfra && !bIsInfra) continue;
            if (aIsInfra && bIsInfra && !rootInfraTypes.has(svcType[a]) && !rootInfraTypes.has(svcType[b])) continue;
            const edgeId = `net-${role}-${a}-${b}`;
            const savedHandles = edgeHandleOverridesRef.current.get(edgeId);
            edges.push({
              id: edgeId,
              source: `service:${a}`,
              target: `service:${b}`,
              sourceHandle: savedHandles?.sourceHandle ?? `iface-${role}`,
              targetHandle: savedHandles?.targetHandle ?? `iface-${role}`,
              type: 'interface',
              data: { role, cidr },
            });
          }
        }
      }

      setRfNodes(nodes);
      setRfEdges(edges);

      // Persist new layout to sidecar
      if (isNewLayout && store.manifestPath) {
        layoutRef.current = layout;
        store.setLayout(layout);
        vscode?.postMessage({ type: 'SAVE_LAYOUT', layout });
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [store.manifestPath, store.layout]
  );

  // ── Node drag end → save layout ───────────────────────────────────────────
  const onNodeDragStop = useCallback(
    (_: React.MouseEvent, node: Node) => {
      const current = layoutRef.current ?? { version: 1 as const, nodes: {} };
      const updated: LayoutData = {
        version: 1,
        nodes: { ...current.nodes, [node.id]: { x: node.position.x, y: node.position.y } },
        edgeHandles: Object.fromEntries(edgeHandleOverridesRef.current),
      };
      layoutRef.current = updated;  // update ref immediately so DOCUMENT_UPDATED sees it
      store.setLayout(updated);
      vscode?.postMessage({ type: 'SAVE_LAYOUT', layout: updated });
    },
    [store]
  );

  // ── Reconnect: drag an edge endpoint to a new chip handle ─────────────────
  // ── Intercept node deletions → write deleteService to YAML ─────────────
  // React Flow fires onEdgesChange BEFORE onNodesChange, so a ref-based approach
  // to skip node-caused edge removes is unreliable. Instead: only apply YAML
  // mutations for NODE deletions here. Edge-side cleanup is handled by
  // deleteService (it removes the whole service including interfaces). Edges
  // visually disappear on canvas re-render from DOCUMENT_UPDATED.

  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => {
      // Forward ALL changes immediately for visual feedback
      onNodesChange(changes);

      const removals = changes.filter(
        (c): c is NodeChange & { type: 'remove' } =>
          c.type === 'remove' && (c.id as string).startsWith('service:'),
      );

      for (const r of removals) {
        const serviceName = r.id.replace('service:', '');
        if (!rawYamlRef.current) continue;

        // Find network roles that will be orphaned after this service is removed.
        const preDel = parse(rawYamlRef.current);
        const orphanedRoles: string[] = [];
        if (!('error' in preDel)) {
          const deletedIfaces = preDel.model.spec.services
            .find(s => s.name === serviceName)?.interfaces ?? [];
          const otherServices = preDel.model.spec.services.filter(s => s.name !== serviceName);
          for (const iface of deletedIfaces) {
            const usedElsewhere = otherServices.some(s => (s.interfaces ?? []).some(i => i.role === iface.role));
            if (!usedElsewhere) orphanedRoles.push(iface.role);
          }
        }

        let { newYaml, description } = applyMutation(rawYamlRef.current, {
          kind: 'deleteService',
          name: serviceName,
        });

        for (const role of orphanedRoles) {
          ({ newYaml, description } = applyMutation(newYaml, { kind: 'deleteNetwork', role }));
        }

        rawYamlRef.current = newYaml;
        vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml, description });
      }
    },
    [onNodesChange],
  );

  // Forward edge changes for visual feedback only — no YAML mutations.
  // Node deletion handles all YAML cleanup via deleteService.
  const handleEdgesChange = useCallback(
    (changes: EdgeChange[]) => { onEdgesChange(changes); },
    [onEdgesChange],
  );

  // ── Reconnect: drag an edge endpoint to rewire an interface ──────────────
  //
  // In ConnectionMode.Loose (all handles are type="source"), React Flow may
  // internally swap source/target, making source/target-based logic unreliable.
  //
  // Robust strategy: find which service was REMOVED from the connection — that is
  // the service whose endpoint was dragged. Use set arithmetic, not source/target order.
  //
  //   bng↔webpa/mgmt: drag webpa to gateway/brlan0  → webpa: mgmt→lan-p1 ✓
  //   client↔gateway/lan-p1: drag gateway to lan-p2  → client: lan-p1→lan-p2 ✓
  const onReconnect = useCallback(
    (oldEdge: Edge, newConnection: Connection) => {
      setRfEdges(eds => reconnectEdge(oldEdge, newConnection, eds));
      edgeHandleOverridesRef.current.set(oldEdge.id, {
        sourceHandle: newConnection.sourceHandle ?? oldEdge.sourceHandle,
        targetHandle: newConnection.targetHandle ?? oldEdge.targetHandle,
      });
      vscode?.postMessage({ type: 'SAVE_LAYOUT', layout: {
        version: 1,
        nodes: layoutRef.current?.nodes ?? {},
        edgeHandles: Object.fromEntries(edgeHandleOverridesRef.current),
      } });

      if (!rawYamlRef.current) return;
      const parsed = parse(rawYamlRef.current);
      if ('error' in parsed) return;
      const currentModel = parsed.model;

      const resolveHandleRole = (handle: string | null | undefined): string | null => {
        if (!handle) return null;
        if (handle.startsWith('iface-') && handle !== 'iface-connect')
          return handle.replace('iface-', '').replace(/-left$/, '');
        return null;
      };

      const oldSrc = oldEdge.source;
      const oldTgt = oldEdge.target;
      const newSrc = newConnection.source;
      const newTgt = newConnection.target;

      // Service that was in the old connection but is absent from the new one.
      const movedFromId =
        (oldSrc !== newSrc && oldSrc !== newTgt) ? oldSrc :
        (oldTgt !== newSrc && oldTgt !== newTgt) ? oldTgt : null;

      let serviceName: string;
      let oldRole: string | null;
      let newRole: string | null;

      // Infrastructure service types. When an infra endpoint is swapped for
      // another infra endpoint (hub swap, e.g. bng→gateway), the CLIENT service
      // that STAYED needs its role updated. When a client moves, it gets updated.
      const infraSvcTypes = new Set(['bng', 'gateway']);

      if (movedFromId) {
        const newDestId = (newSrc !== oldSrc && newSrc !== oldTgt) ? newSrc
                        : (newTgt !== oldSrc && newTgt !== oldTgt) ? newTgt : null;
        const newHandle = (newConnection.source === newDestId) ? newConnection.sourceHandle : newConnection.targetHandle;
        newRole = resolveHandleRole(newHandle);

        const movedSvcType = currentModel.spec.services.find(
          s => `service:${s.name}` === movedFromId
        )?.type ?? '';

        if (infraSvcTypes.has(movedSvcType)) {
          // Hub swap (e.g. bng→gateway): update the CLIENT service that stayed.
          const stayedId = (oldSrc === movedFromId) ? oldTgt : oldSrc;
          serviceName = (stayedId ?? '').replace('service:', '');
          const stayedHandle = (oldEdge.source === stayedId) ? oldEdge.sourceHandle : oldEdge.targetHandle;
          oldRole = resolveHandleRole(stayedHandle);
        } else {
          // Client relocation: the moved client gets the new role.
          serviceName = movedFromId.replace('service:', '');
          const oldHandle = (oldEdge.source === movedFromId) ? oldEdge.sourceHandle : oldEdge.targetHandle;
          oldRole = resolveHandleRole(oldHandle);
        }
      } else {
        // Both endpoints stayed on the same nodes; only a handle changed (hub-and-spoke).
        const srcHandleChanged = oldEdge.sourceHandle !== newConnection.sourceHandle;
        const tgtHandleChanged = oldEdge.targetHandle !== newConnection.targetHandle;
        if (!srcHandleChanged && !tgtHandleChanged) return;

        if (tgtHandleChanged) {
          serviceName = (oldSrc ?? '').replace('service:', '');
          oldRole = resolveHandleRole(oldEdge.sourceHandle);
          newRole = resolveHandleRole(newConnection.targetHandle);
        } else {
          serviceName = (oldTgt ?? '').replace('service:', '');
          oldRole = resolveHandleRole(oldEdge.targetHandle);
          newRole = resolveHandleRole(newConnection.sourceHandle);
        }
      }

      if (!serviceName || !oldRole || !newRole || oldRole === newRole) return;

      const svcIdx = currentModel.spec.services.findIndex(s => s.name === serviceName);
      if (svcIdx < 0) return;
      const ifaceIdx = currentModel.spec.services[svcIdx].interfaces?.findIndex(i => i.role === oldRole) ?? -1;
      if (ifaceIdx < 0) return;

      const { newYaml, description } = applyMutation(rawYamlRef.current, {
        kind: 'setScalar',
        path: ['spec', 'services', svcIdx, 'interfaces', ifaceIdx, 'role'],
        value: newRole,
      });
      rawYamlRef.current = newYaml;
      vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml, description });
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [setRfEdges]
  );

  // ── Connect: drawing a new edge between handles adds an interface ────────
  const onConnect = useCallback(
    (connection: Connection) => {
      if (!rawYamlRef.current) return;
      const parsed = parse(rawYamlRef.current);
      if ('error' in parsed) return;
      const currentModel = parsed.model;

      const srcHandle = connection.sourceHandle ?? '';
      const tgtHandle = connection.targetHandle ?? '';

      const handleToRole = (handle: string): string | null => {
        if (handle.startsWith('iface-') && handle !== 'iface-connect')
          return handle.replace('iface-', '').replace(/-left$/, '');
        return null;
      };

      const role =
        handleToRole(srcHandle) ??
        handleToRole(tgtHandle) ??
        null;

      if (!role) return;

      const srcSvcName = (connection.source ?? '').replace('service:', '');
      const tgtSvcName = (connection.target ?? '').replace('service:', '');
      const receivingSvc = tgtHandle === 'iface-connect' ? tgtSvcName : srcSvcName;

      const svcIdx = currentModel.spec.services.findIndex(s => s.name === receivingSvc);
      if (svcIdx < 0) return;

      const edgeId = `net-${role}-${[srcSvcName, tgtSvcName].sort().join('-')}`;
      edgeHandleOverridesRef.current.set(edgeId, {
        sourceHandle: connection.sourceHandle ?? null,
        targetHandle: connection.targetHandle ?? null,
      });
      vscode?.postMessage({ type: 'SAVE_LAYOUT', layout: {
        version: 1,
        nodes: layoutRef.current?.nodes ?? {},
        edgeHandles: Object.fromEntries(edgeHandleOverridesRef.current),
      } });
      setRfEdges(eds => addEdge({
        ...connection,
        id: edgeId,
        type: 'interface',
        data: { role },
      }, eds));

      if (currentModel.spec.services[svcIdx].interfaces?.some(i => i.role === role)) return;

      const { newYaml, description } = applyMutation(rawYamlRef.current, {
        kind: 'addInterface',
        serviceIndex: svcIdx,
        iface: { role },
      });
      rawYamlRef.current = newYaml;
      vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml, description });
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [setRfEdges]
  );

  // ── Drop from palette ─────────────────────────────────────────────────────
  // window.prompt() returns null in VS Code webviews, so we auto-generate a
  // unique name from the type + count of existing services of that type.
  const onDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      const data = event.dataTransfer.getData('text/plain');
      if (!data) return;
      const payload = JSON.parse(data) as (ServiceTypeDescriptor | PaletteVariant);

      // Resolve the base type name and drop template.
      // Precedence: variant template → serviceDropDefaults override → ExpectedRoles() fallback.
      const isVariant = (payload as PaletteVariant)._variant === true;
      const baseType = isVariant ? (payload as PaletteVariant).type : (payload as ServiceTypeDescriptor).name;
      const typeDesc = store.types.find(t => t.name === baseType);

      type TemplateIface = { role: string; device?: string; bridge?: string; sharing?: 'shared' | 'unique' };
      type Template = { interfaces: TemplateIface[]; bridges?: Array<{ name: string; ipv4?: string }> };

      let template: Template | null = null;
      if (isVariant) {
        const v = payload as PaletteVariant;
        template = { interfaces: v.interfaces, bridges: v.bridges };
      } else if (store.dropDefaults[baseType]) {
        template = store.dropDefaults[baseType];
      } else if (DEFAULT_SERVICE_TEMPLATES[baseType]) {
        template = DEFAULT_SERVICE_TEMPLATES[baseType];
      }

      // Generate a unique name using the base type stem.
      const model = store.model;
      const existing = (model?.spec.services ?? [])
        .map(s => s.name)
        .filter(n => n === baseType || n.startsWith(`${baseType}-`));
      const indices = existing
        .map(n => parseInt(n.replace(`${baseType}-`, ''), 10))
        .filter(n => !isNaN(n));
      const next = indices.length > 0 ? Math.max(...indices) + 1 : 1;
      const name = existing.includes(baseType) || existing.length > 0
        ? `${baseType}-${next}`
        : baseType;

      if (!rawYamlRef.current) return;

      const preDrop = parse(rawYamlRef.current);
      const existingRoles = new Set(
        'error' in preDrop ? [] : preDrop.model.spec.networks.map(n => n.role)
      );

      // Suffix for unique roles: find the smallest N where all unique roles are available,
      // independent of same-type service count (e.g. first xb10 still needs a suffix when
      // lan-p1 already exists from a gateway).
      const suffix = existing.length === 0 ? '' : `-${next}`;

      let currentYaml = rawYamlRef.current;

      if (template) {
        // ── Template path: explicit interface definitions ─────────────────────
        // Compute a role suffix that guarantees all unique roles are free.
        const uniqueRoles = template.interfaces
          .filter(i => (i.sharing ?? 'shared') === 'unique')
          .map(i => i.role);
        let roleSuffix = '';
        if (uniqueRoles.some(r => existingRoles.has(r))) {
          for (let n = 1; ; n++) {
            const candidate = `-${n}`;
            if (uniqueRoles.every(r => !existingRoles.has(`${r}${candidate}`))) {
              roleSuffix = candidate;
              break;
            }
          }
        }

        // Map each template interface to its actual role/bridge name.
        const ifaceMap = template.interfaces.map(iface => {
          const sharing = iface.sharing ?? 'shared';
          const actualRole = sharing === 'unique' ? `${iface.role}${roleSuffix}` : iface.role;
          const actualBridge = (iface.bridge && sharing === 'unique') ? `${iface.bridge}${roleSuffix}` : iface.bridge;
          return { ...iface, actualRole, actualBridge };
        });

        // Create any missing networks for the actual roles.
        for (const iface of ifaceMap) {
          if (existingRoles.has(iface.actualRole)) continue;
          const defaults = DEFAULT_NETWORKS[iface.role];
          if (!defaults) continue;
          // Strip ipv4 from suffixed unique networks to avoid CIDR conflicts.
          const networkConfig = roleSuffix ? { ...defaults, ipv4: undefined } : defaults;
          const { newYaml } = applyMutation(currentYaml, {
            kind: 'insertNetwork',
            network: { role: iface.actualRole, ...networkConfig },
          });
          currentYaml = newYaml;
          existingRoles.add(iface.actualRole);
        }

        // Resolve bridges: apply roleSuffix to any bridge referenced by a unique interface.
        const resolvedBridges = (template.bridges ?? []).map(b => {
          const isUniqueBridge = template!.interfaces.some(
            i => i.bridge === b.name && (i.sharing ?? 'shared') === 'unique'
          );
          return { ...b, name: isUniqueBridge ? `${b.name}${roleSuffix}` : b.name };
        });

        const mutation = applyMutation(currentYaml, {
          kind: 'insertService',
          service: {
            name,
            type: baseType,
            replicas: 1,
            image: { repository: typeDesc?.defaultImage ?? '', tag: 'dev', pullPolicy: 'build-if-missing' },
            interfaces: ifaceMap.map(i => ({
              role: i.actualRole,
              ...(i.device ? { device: i.device } : {}),
              ...(i.actualBridge ? { bridge: i.actualBridge } : {}),
            })),
            ...(resolvedBridges.length > 0 ? { bridges: resolvedBridges } : {}),
          },
        });
        rawYamlRef.current = mutation.newYaml;
        vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml: mutation.newYaml, description: mutation.description });
      } else {
        // ── Fallback path: derive from ExpectedRoles() ────────────────────────
        if (!typeDesc) return;

        // Roles used by bng are upstream shared networks; all others are per-CPE LAN networks.
        const bngRoles = new Set(
          'error' in preDrop ? [] :
          preDrop.model.spec.services
            .filter(s => s.type === 'bng')
            .flatMap(s => (s.interfaces ?? []).map(i => i.role))
        );

        const roleMap: Record<string, string> = {};
        for (const r of typeDesc.expectedRoles) {
          const isShared = bngRoles.has(r.role) || !existingRoles.has(r.role) || suffix === '';
          roleMap[r.role] = isShared ? r.role : `${r.role}${suffix}`;
        }

        for (const r of typeDesc.expectedRoles) {
          const actualRole = roleMap[r.role];
          if (existingRoles.has(actualRole)) continue;
          const defaults = DEFAULT_NETWORKS[r.role];
          if (!defaults) continue;
          const { newYaml } = applyMutation(currentYaml, {
            kind: 'insertNetwork',
            network: { role: actualRole, ...defaults },
          });
          currentYaml = newYaml;
          existingRoles.add(actualRole);
        }

        const mutation = applyMutation(currentYaml, {
          kind: 'insertService',
          service: {
            name,
            type: baseType,
            replicas: 1,
            image: { repository: typeDesc.defaultImage, tag: 'dev', pullPolicy: 'build-if-missing' },
            interfaces: typeDesc.expectedRoles.map(r => ({ role: roleMap[r.role] })),
          },
        });
        rawYamlRef.current = mutation.newYaml;
        vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml: mutation.newYaml, description: mutation.description });
      }
    },
    [store.model, store.types, store.dropDefaults]
  );

  // ── Render ────────────────────────────────────────────────────────────────
  if (!store.manifestPath && !store.model) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
        <WelcomeScreen entries={[]} />
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      {/* Toolbar */}
      <div style={toolbarStyle}>
        <ManifestDropdown currentPath={store.manifestPath} />
        <button
          onClick={store.toggleDependsOn}
          style={{ ...btnStyle, opacity: store.showDependsOn ? 1 : 0.5 }}
          title="Toggle dependency arrows"
        >
          ⇢ Dependencies
        </button>
      </div>

      {/* Canvas area */}
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        <TypePalette types={store.types} typesError={store.typesError} paletteVariants={store.paletteVariants} />

        {/* Error overlay */}
        {store.yamlError ? (
          <div style={errorOverlayStyle}>
            <div style={errorBoxStyle}>
              <strong>⚠ Cannot render manifest</strong>
              <p style={{ marginTop: 8, fontSize: 12 }}>
                {store.yamlError}
                {store.yamlErrorLine && ` (line ${store.yamlErrorLine})`}
              </p>
              <button
                style={{ marginTop: 12, ...btnStyle }}
                onClick={() => vscode?.postMessage({ type: 'OPEN_MANIFEST', path: store.manifestPath })}
              >
                Open in Text Editor
              </button>
            </div>
          </div>
        ) : (
          <div style={{ flex: 1 }}>
            <ReactFlow
              nodes={rfNodes}
              edges={rfEdges}
              nodeTypes={nodeTypes}
              edgeTypes={edgeTypes}
              onNodesChange={handleNodesChange}
              onEdgesChange={handleEdgesChange}
              onNodeClick={(_, node) => store.selectNode(node.id)}
              onPaneClick={() => store.selectNode(null)}
              onNodeDragStop={onNodeDragStop}
              onReconnect={onReconnect}
              onConnect={onConnect}
              reconnectRadius={20}
              connectionMode={ConnectionMode.Loose}
              onDrop={onDrop}
              onDragOver={(e) => e.preventDefault()}
              proOptions={{ hideAttribution: true }}
              fitView
            >
              <Background />
              <Controls />
            </ReactFlow>
          </div>
        )}

        <PropertyPanel
          model={store.model}
          selectedNodeId={store.selectedNodeId}
          onMutation={(description, newYaml) => {
            vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml, description });
          }}
          rawYaml={rawYamlRef.current}
        />
      </div>
    </div>
  );
}

const toolbarStyle: React.CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 10, padding: '6px 12px',
  borderBottom: '1px solid #333', background: '#252526', minHeight: 38,
};
const btnStyle: React.CSSProperties = {
  padding: '3px 10px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
  background: '#333', color: '#ccc', border: '1px solid #555',
};
const errorOverlayStyle: React.CSSProperties = {
  flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center',
  background: 'var(--vscode-editor-background)',
};
const errorBoxStyle: React.CSSProperties = {
  padding: 24, maxWidth: 480, textAlign: 'center',
  border: '1px solid #E74C3C66', borderRadius: 8, background: '#E74C3C11',
};
