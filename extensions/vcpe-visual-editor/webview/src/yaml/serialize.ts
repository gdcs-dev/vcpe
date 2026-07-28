import { Document, isSeq, isMap, parseDocument, stringify } from 'yaml';
import type { Network, Service, Interface, BridgeSpec } from './parse';

// ─── Mutation types ───────────────────────────────────────────────────────────

export type Mutation =
  | { kind: 'setScalar'; path: (string | number)[]; value: unknown }
  | { kind: 'insertNetwork'; network: Network }
  | { kind: 'insertService'; service: Service }
  | { kind: 'deleteNetwork'; role: string }
  | { kind: 'deleteService'; name: string }
  | { kind: 'addInterface'; serviceIndex: number; iface: Interface }
  | { kind: 'removeInterface'; serviceIndex: number; ifaceIndex: number }
  | { kind: 'insertBridge'; serviceIndex: number; bridge: BridgeSpec }
  | { kind: 'deleteBridge'; serviceIndex: number; bridgeIndex: number }
  | { kind: 'renameService'; oldName: string; newName: string }
  | { kind: 'setConfig'; serviceIndex: number; configYaml: string };

// ─── ApplyResult ─────────────────────────────────────────────────────────────

export interface ApplyResult {
  newYaml: string;
  description: string;
}

/**
 * applyMutation applies a single canvas mutation to the YAML text and returns
 * the new YAML string with comments and formatting preserved as much as possible.
 */
export function applyMutation(yamlText: string, mutation: Mutation): ApplyResult {
  const doc = parseDocument(yamlText, { strict: false });

  switch (mutation.kind) {
    case 'setScalar':
      return applySetScalar(doc, mutation.path, mutation.value);

    case 'insertNetwork':
      return applyInsertNetwork(doc, mutation.network);

    case 'insertService':
      return applyInsertService(doc, mutation.service);

    case 'deleteNetwork':
      return applyDeleteNetwork(doc, mutation.role);

    case 'deleteService':
      return applyDeleteService(doc, mutation.name);

    case 'addInterface':
      return applyAddInterface(doc, mutation.serviceIndex, mutation.iface);

    case 'removeInterface':
      return applyRemoveInterface(doc, mutation.serviceIndex, mutation.ifaceIndex);

    case 'insertBridge':
      return applyInsertBridge(doc, mutation.serviceIndex, mutation.bridge);

    case 'deleteBridge':
      return applyDeleteBridge(doc, mutation.serviceIndex, mutation.bridgeIndex);

    case 'renameService':
      return applyRenameService(doc, mutation.oldName, mutation.newName);

    case 'setConfig':
      return applySetConfig(doc, mutation.serviceIndex, mutation.configYaml);
  }
}

// ─── Easy mutations ───────────────────────────────────────────────────────────

function applySetScalar(doc: Document, path: (string | number)[], value: unknown): ApplyResult {
  if (value === null || value === undefined) {
    doc.deleteIn(path);
    // Walk up and prune any ancestor maps that became empty after the deletion.
    pruneEmptyAncestors(doc, path.slice(0, -1));
  } else {
    doc.setIn(path, value);
  }
  return {
    newYaml: String(doc),
    description: `set ${path.join('.')} = ${JSON.stringify(value)}`,
  };
}

/**
 * pruneEmptyAncestors removes a map node at `path` if it has no remaining
 * keys, then recurses up the path. This keeps the YAML tidy after nullable
 * fields (e.g. pool.start + pool.end) are cleared.
 */
function pruneEmptyAncestors(doc: Document, path: (string | number)[]): void {
  if (path.length === 0) return;
  const node = doc.getIn(path);
  if (isMap(node) && node.items.length === 0) {
    doc.deleteIn(path);
    pruneEmptyAncestors(doc, path.slice(0, -1));
  }
}

/**
 * stripNulls recursively removes null/undefined values from a plain object
 * so that doc.createNode() does not serialise them as YAML null.
 */
function stripNulls(obj: unknown): unknown {
  if (obj === null || obj === undefined) return undefined;
  if (Array.isArray(obj)) return obj.map(stripNulls).filter(v => v !== undefined);
  if (typeof obj === 'object') {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
      const cleaned = stripNulls(v);
      if (cleaned !== undefined) out[k] = cleaned;
    }
    return out;
  }
  return obj;
}

function applyAddInterface(doc: Document, serviceIndex: number, iface: Interface): ApplyResult {
  const ifacesPath = ['spec', 'services', serviceIndex, 'interfaces'];
  let interfaces = doc.getIn(ifacesPath);
  if (!isSeq(interfaces)) {
    doc.setIn(ifacesPath, doc.createNode([]));
    interfaces = doc.getIn(ifacesPath);
  }
  const ifaceNode = doc.createNode(iface);
  doc.addIn(ifacesPath, ifaceNode);
  return {
    newYaml: String(doc),
    description: `add interface role=${iface.role} to services[${serviceIndex}]`,
  };
}

function applyRemoveInterface(doc: Document, serviceIndex: number, ifaceIndex: number): ApplyResult {
  doc.deleteIn(['spec', 'services', serviceIndex, 'interfaces', ifaceIndex]);
  return {
    newYaml: String(doc),
    description: `remove interfaces[${ifaceIndex}] from services[${serviceIndex}]`,
  };
}

function applyInsertBridge(doc: Document, serviceIndex: number, bridge: BridgeSpec): ApplyResult {
  const bridgesPath = ['spec', 'services', serviceIndex, 'bridges'];
  if (!isSeq(doc.getIn(bridgesPath))) {
    doc.setIn(bridgesPath, doc.createNode([]));
  }
  doc.addIn(bridgesPath, doc.createNode(stripNulls(bridge)));
  return {
    newYaml: String(doc),
    description: `add bridge ${bridge.name} to services[${serviceIndex}]`,
  };
}

function applyDeleteBridge(doc: Document, serviceIndex: number, bridgeIndex: number): ApplyResult {
  doc.deleteIn(['spec', 'services', serviceIndex, 'bridges', bridgeIndex]);
  pruneEmptyAncestors(doc, ['spec', 'services', serviceIndex, 'bridges']);
  return {
    newYaml: String(doc),
    description: `delete bridges[${bridgeIndex}] from services[${serviceIndex}]`,
  };
}

function applySetConfig(doc: Document, serviceIndex: number, configYaml: string): ApplyResult {
  const configDoc = parseDocument(configYaml, { strict: false });
  doc.setIn(['spec', 'services', serviceIndex, 'config'], configDoc.contents);
  return {
    newYaml: String(doc),
    description: `update config for services[${serviceIndex}]`,
  };
}

// ─── Medium mutations ─────────────────────────────────────────────────────────

function applyInsertNetwork(doc: Document, network: Network): ApplyResult {
  const networksPath = ['spec', 'networks'];
  let networks = doc.getIn(networksPath);
  if (!isSeq(networks)) {
    doc.setIn(networksPath, doc.createNode([]));
    networks = doc.getIn(networksPath);
  }
  doc.addIn(networksPath, doc.createNode(stripNulls(network)));
  return {
    newYaml: String(doc),
    description: `insert network role=${network.role}`,
  };
}

function applyInsertService(doc: Document, service: Service): ApplyResult {
  const servicesPath = ['spec', 'services'];
  let services = doc.getIn(servicesPath);
  if (!isSeq(services)) {
    doc.setIn(servicesPath, doc.createNode([]));
    services = doc.getIn(servicesPath);
  }
  doc.addIn(servicesPath, doc.createNode(stripNulls(service)));
  return {
    newYaml: String(doc),
    description: `insert service name=${service.name}`,
  };
}

// ─── Hard mutations (cross-reference cleanup) ─────────────────────────────────

/**
 * applyDeleteService removes a service by name AND scrubs all dependsOn
 * references to that service name from every other service.
 * Both operations are applied to the same Document so the result is atomic.
 */
function applyDeleteService(doc: Document, name: string): ApplyResult {
  const servicesNode = doc.getIn(['spec', 'services']);
  if (!isSeq(servicesNode)) {
    return { newYaml: String(doc), description: `delete service ${name} (not found)` };
  }

  // Find and remove the target service
  const serviceIndex = servicesNode.items.findIndex(
    (item) => isMap(item) && item.get('name') === name
  );
  if (serviceIndex >= 0) {
    doc.deleteIn(['spec', 'services', serviceIndex]);
  }

  // Re-fetch after potential index shift; clean dependsOn across all remaining services
  const updatedServices = doc.getIn(['spec', 'services']);
  if (isSeq(updatedServices)) {
    updatedServices.items.forEach((svcNode, svcIdx) => {
      if (!isMap(svcNode)) return;
      const depsNode = svcNode.get('dependsOn', true);
      if (!isSeq(depsNode)) return;
      const refIndex = depsNode.items.findIndex(
        (d) => String((d as { value?: unknown }).value ?? d) === name
      );
      if (refIndex >= 0) {
        doc.deleteIn(['spec', 'services', svcIdx, 'dependsOn', refIndex]);
      }
    });
  }

  return {
    newYaml: String(doc),
    description: `delete service ${name} and clean dependsOn cross-references`,
  };
}

/**
 * applyRenameService updates a service's name AND rewrites all dependsOn
 * references to that name across every other service, atomically.
 */
function applyRenameService(doc: Document, oldName: string, newName: string): ApplyResult {
  if (!newName.trim() || newName === oldName) {
    return { newYaml: String(doc), description: 'rename service (no-op)' };
  }
  const servicesNode = doc.getIn(['spec', 'services']);
  if (!isSeq(servicesNode)) {
    return { newYaml: String(doc), description: `rename service ${oldName} (not found)` };
  }

  // Rename the service itself
  const serviceIndex = servicesNode.items.findIndex(
    (item) => isMap(item) && item.get('name') === oldName
  );
  if (serviceIndex >= 0) {
    doc.setIn(['spec', 'services', serviceIndex, 'name'], newName);
  }

  // Update dependsOn references in all services
  servicesNode.items.forEach((svcNode, svcIdx) => {
    if (!isMap(svcNode)) return;
    const depsNode = svcNode.get('dependsOn', true);
    if (!isSeq(depsNode)) return;
    depsNode.items.forEach((d, dIdx) => {
      const val = String((d as { value?: unknown }).value ?? d);
      if (val === oldName) {
        doc.setIn(['spec', 'services', svcIdx, 'dependsOn', dIdx], newName);
      }
    });
  });

  return {
    newYaml: String(doc),
    description: `rename service ${oldName} → ${newName} and update dependsOn references`,
  };
}

/**
 * applyDeleteNetwork removes a network by role AND removes all interface entries
 * referencing that role from every service in the manifest.
 */
function applyDeleteNetwork(doc: Document, role: string): ApplyResult {
  const networksNode = doc.getIn(['spec', 'networks']);
  if (isSeq(networksNode)) {
    const netIndex = networksNode.items.findIndex(
      (item) => isMap(item) && item.get('role') === role
    );
    if (netIndex >= 0) {
      doc.deleteIn(['spec', 'networks', netIndex]);
    }
  }

  // Remove all interface entries referencing this role from every service
  const servicesNode = doc.getIn(['spec', 'services']);
  if (isSeq(servicesNode)) {
    servicesNode.items.forEach((svcNode, svcIdx) => {
      if (!isMap(svcNode)) return;
      const ifacesNode = svcNode.get('interfaces', true);
      if (!isSeq(ifacesNode)) return;
      // Walk backwards to safely delete by index
      for (let i = ifacesNode.items.length - 1; i >= 0; i--) {
        const iface = ifacesNode.items[i];
        if (isMap(iface) && iface.get('role') === role) {
          doc.deleteIn(['spec', 'services', svcIdx, 'interfaces', i]);
        }
      }
    });
  }

  return {
    newYaml: String(doc),
    description: `delete network role=${role} and clean interface cross-references`,
  };
}
