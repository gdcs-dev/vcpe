import React, { useCallback, useEffect, useRef } from 'react';
import {
  ReactFlow, Background, Controls, MiniMap,
  useNodesState, useEdgesState, reconnectEdge,
  type Node, type Edge, type Connection, type NodeChange, type EdgeChange,
  MarkerType, ConnectionMode, addEdge,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { parse, type ParseResult } from './yaml/parse';
import { applyMutation } from './yaml/serialize';
import { computeInitialLayout } from './layout/autoLayout';
import { useManifestStore } from './store/manifestStore';

import { ServiceNode, type ServiceNodeData, type BridgeGroup } from './nodes/ServiceNode';
import { InterfaceEdge } from './edges/InterfaceEdge';
import { DependsOnEdge } from './edges/DependsOnEdge';

import { TypePalette } from './panels/TypePalette';
import { PropertyPanel } from './panels/PropertyPanel';
import { ManifestDropdown } from './panels/ManifestDropdown';
import { WelcomeScreen } from './panels/WelcomeScreen';

import type { ServiceTypeDescriptor, LayoutData } from './types';
import type { Network } from './yaml/parse';

import { vscodeApi as vscode } from './vsCodeApi';

// ─── Default network settings (based on example.yaml) ────────────────────────
// Auto-created when a service is dropped and these roles don't exist yet.
const DEFAULT_NETWORKS: Record<string, Omit<Network, 'role'>> = {
  'mgmt':  { ipv4: { cidr: '10.10.10.0/24',  gateway: '10.10.10.1',  pool: { start: '10.10.10.10',  end: '10.10.10.250'  } } },
  'wan':   { nat: true, firewall: true, ipamDriver: 'none', ipv4: { cidr: '10.7.200.0/24', gateway: '10.7.200.1', pool: { start: '10.7.200.10', end: '10.7.200.250' } } },
  'cm':    { ipamDriver: 'none', ipv4: { cidr: '10.7.201.0/24', gateway: '10.7.201.1', pool: { start: '10.7.201.10', end: '10.7.201.250' } } },
  'lan-p1':{ ipamDriver: 'none', ipv4: { cidr: '192.168.10.0/24', gateway: '192.168.10.1', pool: { start: '192.168.10.10', end: '192.168.10.250' } } },
  'lan-p2':{ ipamDriver: 'none', ipv4: { cidr: '192.168.20.0/24', gateway: '192.168.20.1', pool: { start: '192.168.20.10', end: '192.168.20.250' } } },
  'lan-p3':{ ipamDriver: 'none', ipv4: { cidr: '192.168.30.0/24', gateway: '192.168.30.1', pool: { start: '192.168.30.10', end: '192.168.30.250' } } },
  'lan-p4':{ ipamDriver: 'none', ipv4: { cidr: '192.168.40.0/24', gateway: '192.168.40.1', pool: { start: '192.168.40.10', end: '192.168.40.250' } } },
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

  // ── Extension message handler ──────────────────────────────────────────────
  useEffect(() => {
    const handler = (event: MessageEvent) => {
      const msg = event.data;
      if (!msg?.type) return;

      switch (msg.type) {
        case 'INIT': {
          rawYamlRef.current = msg.yaml ?? '';
          store.setTypes(msg.types ?? [], msg.typesError ?? null);
          store.setManifestPath(msg.manifestPath ?? null);
          if (msg.layout) {
            layoutRef.current = msg.layout as LayoutData;
            store.setLayout(msg.layout as LayoutData);
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
        const bridgedRoles = new Set((svc.interfaces ?? []).filter(i => i.bridge).map(i => i.role));
        const nonBridgedIfaces = (svc.interfaces ?? []).filter(i => !i.bridge);

        // Build bridge groups: one per declared bridge, with member interfaces.
        const bridgeGroups: BridgeGroup[] = (svc.bridges ?? []).map(b => ({
          name: b.name,
          ipv4: b.ipv4,
          members: (svc.interfaces ?? [])
            .filter(i => i.bridge === b.name)
            .map(i => ({
              role: i.role,
              device: i.device,
              bridge: i.bridge,
              ipv4: i.ipv4,
              defaultRoute: i.defaultRoute,
            })),
        }));

        nodes.push({
          id: nodeId,
          type: 'service',
          position: pos(nodeId),
          data: {
            name: svc.name,
            type: svc.type,
            replicas: svc.replicas,
            networks: nonBridgedIfaces.map(i => ({
              role: i.role,
              device: i.device,
              bridge: i.bridge,
              ipv4: i.ipv4,
              defaultRoute: i.defaultRoute,
            })),
            bridges: bridgeGroups,
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
      // When a service's interface has bridge: brlan0, the edge connects to the
      // bridge handle (iface-bridge-brlan0) rather than iface-{role}, so the
      // connection visually terminates at the bridge section of the service node.
      const svcBridgeForRole = new Map<string, string>(); // "svcName:role" → bridgeName
      for (const svc of model.spec.services) {
        for (const iface of svc.interfaces ?? []) {
          if (iface.bridge) svcBridgeForRole.set(`${svc.name}:${iface.role}`, iface.bridge);
        }
      }
      const resolveHandle = (svcName: string, role: string): string => {
        const bridge = svcBridgeForRole.get(`${svcName}:${role}`);
        return bridge ? `iface-bridge-${bridge}` : `iface-${role}`;
      };

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
      const infraTypes = new Set(['bng', 'gateway']);

      for (const [role, svcNames] of Object.entries(netServices)) {
        if (svcNames.length < 2) continue;
        const net = model.spec.networks.find(n => n.role === role);
        const cidr = net?.ipv4?.cidr ?? net?.ipv6?.cidr;
        for (let i = 0; i < svcNames.length; i++) {
          for (let j = i + 1; j < svcNames.length; j++) {
            const [a, b] = [svcNames[i], svcNames[j]].sort();
            if (!infraTypes.has(svcType[a]) && !infraTypes.has(svcType[b])) continue;
            edges.push({
              id: `net-${role}-${a}-${b}`,
              source: `service:${a}`,
              target: `service:${b}`,
              sourceHandle: resolveHandle(a, role),
              targetHandle: resolveHandle(b, role),
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
        const { newYaml, description } = applyMutation(rawYamlRef.current, {
          kind: 'deleteService',
          name: serviceName,
        });
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

      if (!rawYamlRef.current) return;
      const parsed = parse(rawYamlRef.current);
      if ('error' in parsed) return;
      const currentModel = parsed.model;

      const resolveHandleRole = (handle: string | null | undefined, serviceId: string | null | undefined): string | null => {
        if (!handle) return null;
        if (handle.startsWith('iface-bridge-')) {
          const bridgeName = handle.replace('iface-bridge-', '');
          const svcName = (serviceId ?? '').replace('service:', '');
          const svc = currentModel.spec.services.find(s => s.name === svcName);
          return svc?.interfaces?.find(i => i.bridge === bridgeName)?.role ?? null;
        }
        if (handle.startsWith('iface-') && handle !== 'iface-connect') return handle.replace('iface-', '');
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
        newRole = resolveHandleRole(newHandle, newDestId);

        const movedSvcType = currentModel.spec.services.find(
          s => `service:${s.name}` === movedFromId
        )?.type ?? '';

        if (infraSvcTypes.has(movedSvcType)) {
          // Hub swap (e.g. bng→gateway): update the CLIENT service that stayed.
          const stayedId = (oldSrc === movedFromId) ? oldTgt : oldSrc;
          serviceName = (stayedId ?? '').replace('service:', '');
          const stayedHandle = (oldEdge.source === stayedId) ? oldEdge.sourceHandle : oldEdge.targetHandle;
          oldRole = resolveHandleRole(stayedHandle, stayedId);
        } else {
          // Client relocation: the moved client gets the new role.
          serviceName = movedFromId.replace('service:', '');
          const oldHandle = (oldEdge.source === movedFromId) ? oldEdge.sourceHandle : oldEdge.targetHandle;
          oldRole = resolveHandleRole(oldHandle, movedFromId);
        }
      } else {
        // Both endpoints stayed on the same nodes; only a handle changed (hub-and-spoke).
        const srcHandleChanged = oldEdge.sourceHandle !== newConnection.sourceHandle;
        const tgtHandleChanged = oldEdge.targetHandle !== newConnection.targetHandle;
        if (!srcHandleChanged && !tgtHandleChanged) return;

        if (tgtHandleChanged) {
          serviceName = (oldSrc ?? '').replace('service:', '');
          oldRole = resolveHandleRole(oldEdge.sourceHandle, oldSrc);
          newRole = resolveHandleRole(newConnection.targetHandle, newTgt);
        } else {
          serviceName = (oldTgt ?? '').replace('service:', '');
          oldRole = resolveHandleRole(oldEdge.targetHandle, oldTgt);
          newRole = resolveHandleRole(newConnection.sourceHandle, newSrc);
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

      // Resolve handle to network role, including bridge handles.
      const handleToRole = (handle: string, serviceId: string): string | null => {
        if (handle.startsWith('iface-bridge-')) {
          const bridgeName = handle.replace('iface-bridge-', '');
          const svcName = serviceId.replace('service:', '');
          const svc = currentModel.spec.services.find(s => s.name === svcName);
          return svc?.interfaces?.find(i => i.bridge === bridgeName)?.role ?? null;
        }
        if (handle.startsWith('iface-') && handle !== 'iface-connect') {
          return handle.replace('iface-', '');
        }
        return null;
      };

      const role =
        handleToRole(srcHandle, connection.source ?? '') ??
        handleToRole(tgtHandle, connection.target ?? '') ??
        null;

      if (!role) return;

      const srcSvcName = (connection.source ?? '').replace('service:', '');
      const tgtSvcName = (connection.target ?? '').replace('service:', '');
      const receivingSvc = tgtHandle === 'iface-connect' ? tgtSvcName : srcSvcName;

      const svcIdx = currentModel.spec.services.findIndex(s => s.name === receivingSvc);
      if (svcIdx < 0) return;

      setRfEdges(eds => addEdge({
        ...connection,
        id: `net-${role}-${[srcSvcName, tgtSvcName].sort().join('-')}`,
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
      const typeDesc: ServiceTypeDescriptor = JSON.parse(data);

      // Generate a unique name: <type>-<n> where n is one above the highest existing index.
      const model = store.model;
      const existing = (model?.spec.services ?? [])
        .map(s => s.name)
        .filter(n => n === typeDesc.name || n.startsWith(`${typeDesc.name}-`));
      const indices = existing
        .map(n => parseInt(n.replace(`${typeDesc.name}-`, ''), 10))
        .filter(n => !isNaN(n));
      const next = indices.length > 0 ? Math.max(...indices) + 1 : 1;
      const name = existing.includes(typeDesc.name) || existing.length > 0
        ? `${typeDesc.name}-${next}`
        : typeDesc.name;

      if (!rawYamlRef.current) return;

      // Parse current YAML to find existing networks
      const preDrop = parse(rawYamlRef.current);
      const existingRoles = new Set(
        'error' in preDrop ? [] : preDrop.model.spec.networks.map(n => n.role)
      );

      let currentYaml = rawYamlRef.current;

      // Create any missing networks for this service's expected roles using defaults
      for (const r of typeDesc.expectedRoles) {
        if (existingRoles.has(r.role)) continue;
        const defaults = DEFAULT_NETWORKS[r.role];
        if (!defaults) continue;
        const { newYaml } = applyMutation(currentYaml, {
          kind: 'insertNetwork',
          network: { role: r.role, ...defaults },
        });
        currentYaml = newYaml;
        existingRoles.add(r.role);
      }

      const mutation = applyMutation(currentYaml, {
        kind: 'insertService',
        service: {
          name,
          type: typeDesc.name,
          replicas: 1,
          image: { repository: typeDesc.defaultImage, tag: 'dev', pullPolicy: 'build-if-missing' },
          interfaces: typeDesc.expectedRoles.map(r => ({ role: r.role })),
        },
      });
      rawYamlRef.current = mutation.newYaml;
      vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml: mutation.newYaml, description: mutation.description });
    },
    [store.model]
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
        <TypePalette types={store.types} typesError={store.typesError} />

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
              fitView
            >
              <Background />
              <Controls />
              <MiniMap />
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
