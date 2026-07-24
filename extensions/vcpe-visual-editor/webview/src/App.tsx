import React, { useCallback, useEffect, useRef } from 'react';
import {
  ReactFlow, Background, Controls, MiniMap,
  useNodesState, useEdgesState, reconnectEdge,
  type Node, type Edge, type Connection, type NodeChange, type EdgeChange,
  MarkerType, ConnectionMode,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { parse, type ParseResult } from './yaml/parse';
import { applyMutation } from './yaml/serialize';
import { computeInitialLayout } from './layout/autoLayout';
import { useManifestStore } from './store/manifestStore';

import { ServiceNode, type ServiceNodeData } from './nodes/ServiceNode';
import { InterfaceEdge } from './edges/InterfaceEdge';
import { DependsOnEdge } from './edges/DependsOnEdge';

import { TypePalette } from './panels/TypePalette';
import { PropertyPanel } from './panels/PropertyPanel';
import { ManifestDropdown } from './panels/ManifestDropdown';
import { WelcomeScreen } from './panels/WelcomeScreen';

import type { ServiceTypeDescriptor, LayoutData } from './types';

import { vscodeApi as vscode } from './vsCodeApi';

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
        nodes.push({
          id: nodeId,
          type: 'service',
          position: pos(nodeId),
          data: {
            name: svc.name,
            type: svc.type,
            replicas: svc.replicas,
            networks: svc.interfaces?.map(i => ({
              role: i.role,
              device: i.device,
              ipv4: i.ipv4,
              defaultRoute: i.defaultRoute,
            })) ?? [],
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
      // Only draw edges where at least one end is infrastructure — this prevents
      // peer-client edges (e.g. webpa ↔ event-sink both on mgmt).
      const infraTypes = new Set(['bng', 'gateway']);

      for (const [role, svcNames] of Object.entries(netServices)) {
        if (svcNames.length < 2) continue;
        const net = model.spec.networks.find(n => n.role === role);
        const cidr = net?.ipv4?.cidr ?? net?.ipv6?.cidr;
        for (let i = 0; i < svcNames.length; i++) {
          for (let j = i + 1; j < svcNames.length; j++) {
            const [a, b] = [svcNames[i], svcNames[j]].sort();
            // Skip pure client-to-client edges
            if (!infraTypes.has(svcType[a]) && !infraTypes.has(svcType[b])) continue;
            edges.push({
              id: `net-${role}-${a}-${b}`,
              source: `service:${a}`,
              target: `service:${b}`,
              sourceHandle: `iface-${role}`,
              targetHandle: `iface-${role}`,
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
  // An edge represents two services sharing a network (same role on both ends).
  // The original SOURCE service is the one that gets rewired — both when dragging
  // the target end (to a different port on the hub service) and when dragging the
  // source end to a completely different service/network.
  //
  //   edge: client/iface-lan-p1 ↔ gateway/iface-lan-p1
  //   drag gateway end → gateway/iface-lan-p2  →  client: lan-p1 → lan-p2
  //   drag gateway end → bng/iface-wan          →  client: lan-p1 → wan
  //   drag client end  → bng/iface-wan           →  client: lan-p1 → wan
  const onReconnect = useCallback(
    (oldEdge: Edge, newConnection: Connection) => {
      // Optimistic visual update — prevents the edge snapping back while
      // the YAML mutation round-trips through the extension host.
      setRfEdges(eds => reconnectEdge(oldEdge, newConnection, eds));

      const model = store.model;
      if (!model || !rawYamlRef.current) return;

      const extractRole = (h: string | null | undefined) =>
        h?.replace('iface-', '') ?? '';

      const targetMoved =
        oldEdge.target !== newConnection.target ||
        oldEdge.targetHandle !== newConnection.targetHandle;
      const sourceMoved =
        oldEdge.source !== newConnection.source ||
        oldEdge.sourceHandle !== newConnection.sourceHandle;

      if (!targetMoved && !sourceMoved) return;

      // Always rewire the original source service. The new role is taken from
      // whichever end moved.
      const serviceName = (oldEdge.source ?? '').replace('service:', '');
      const oldRole = extractRole(oldEdge.sourceHandle);
      const newRole = targetMoved
        ? extractRole(newConnection.targetHandle)
        : extractRole(newConnection.sourceHandle);

      if (!serviceName || !oldRole || !newRole || oldRole === newRole) return;

      const svcIdx = model.spec.services.findIndex(s => s.name === serviceName);
      if (svcIdx < 0) return;
      const ifaceIdx =
        model.spec.services[svcIdx].interfaces?.findIndex(i => i.role === oldRole) ?? -1;
      if (ifaceIdx < 0) return;

      const { newYaml, description } = applyMutation(rawYamlRef.current, {
        kind: 'setScalar',
        path: ['spec', 'services', svcIdx, 'interfaces', ifaceIdx, 'role'],
        value: newRole,
      });
      vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml, description });
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [store.model, setRfEdges]
  );

  // ── Drop from palette ─────────────────────────────────────────────────────
  const onDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      const data = event.dataTransfer.getData('application/vcpe-service-type');
      if (!data) return;
      const typeDesc: ServiceTypeDescriptor = JSON.parse(data);
      const name = window.prompt(`Service name for type "${typeDesc.name}":`);
      if (!name) return;

      const mutation = applyMutation(rawYamlRef.current, {
        kind: 'insertService',
        service: {
          name,
          type: typeDesc.name,
          replicas: 1,
          image: { repository: typeDesc.defaultImage },
          interfaces: typeDesc.expectedRoles
            .filter(r => r.required)
            .map(r => ({ role: r.role })),
        },
      });
      vscode?.postMessage({ type: 'CANVAS_MUTATION', newYaml: mutation.newYaml, description: mutation.description });
    },
    []
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
          <div
            style={{ flex: 1 }}
            onDrop={onDrop}
            onDragOver={(e) => e.preventDefault()}
          >
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
              reconnectRadius={20}
              connectionMode={ConnectionMode.Loose}
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
