import dagre from '@dagrejs/dagre';
import type { ManifestModel } from '../yaml/parse';
import type { LayoutData } from '../types';

const SERVICE_WIDTH  = 200;
const SERVICE_HEIGHT = 90;   // base; grows with interface count

/**
 * computeInitialLayout produces canvas node positions for a manifest that
 * has no existing sidecar. Services are laid out by dagre using both
 * dependsOn edges and network-sharing edges so services on the same
 * network cluster together.
 */
export function computeInitialLayout(model: ManifestModel): LayoutData {
  const nodes: LayoutData['nodes'] = {};

  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: 'LR', ranksep: 140, nodesep: 60 });
  g.setDefaultEdgeLabel(() => ({}));

  for (const svc of model.spec.services) {
    const h = SERVICE_HEIGHT + (svc.interfaces?.length ?? 0) * 20;
    g.setNode(`service:${svc.name}`, { width: SERVICE_WIDTH, height: h });
  }

  // Add dependsOn edges (A → B = "A needs B")
  for (const svc of model.spec.services) {
    for (const dep of svc.dependsOn ?? []) {
      if (model.spec.services.some(s => s.name === dep)) {
        g.setEdge(`service:${svc.name}`, `service:${dep}`);
      }
    }
  }

  // Add network-sharing edges to group services that share a network.
  // Only connect adjacent pairs to avoid too many dagre edges.
  const networkServices: Record<string, string[]> = {};
  for (const svc of model.spec.services) {
    for (const iface of svc.interfaces ?? []) {
      if (!networkServices[iface.role]) networkServices[iface.role] = [];
      if (!networkServices[iface.role].includes(svc.name)) {
        networkServices[iface.role].push(svc.name);
      }
    }
  }
  for (const svcNames of Object.values(networkServices)) {
    for (let i = 0; i < svcNames.length - 1; i++) {
      const a = `service:${svcNames[i]}`;
      const b = `service:${svcNames[i + 1]}`;
      if (!g.hasEdge(a, b) && !g.hasEdge(b, a)) {
        g.setEdge(a, b);
      }
    }
  }

  dagre.layout(g);

  for (const svc of model.spec.services) {
    const nodeId = `service:${svc.name}`;
    const n = g.node(nodeId);
    if (n) {
      nodes[nodeId] = { x: n.x - n.width / 2, y: n.y - n.height / 2 };
    }
  }

  return { version: 1, nodes };
}
