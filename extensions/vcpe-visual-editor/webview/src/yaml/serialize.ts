import { Document, isSeq, isMap, parseDocument, stringify } from 'yaml';
import type { Network, Service, Interface } from './parse';

// ─── Mutation types ───────────────────────────────────────────────────────────

export type Mutation =
  | { kind: 'setScalar'; path: (string | number)[]; value: unknown }
  | { kind: 'insertNetwork'; network: Network }
  | { kind: 'insertService'; service: Service }
  | { kind: 'deleteNetwork'; role: string }
  | { kind: 'deleteService'; name: string }
  | { kind: 'addInterface'; serviceIndex: number; iface: Interface }
  | { kind: 'removeInterface'; serviceIndex: number; ifaceIndex: number }
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

    case 'setConfig':
      return applySetConfig(doc, mutation.serviceIndex, mutation.configYaml);
  }
}

// ─── Easy mutations ───────────────────────────────────────────────────────────

function applySetScalar(doc: Document, path: (string | number)[], value: unknown): ApplyResult {
  doc.setIn(path, value);
  return {
    newYaml: String(doc),
    description: `set ${path.join('.')} = ${JSON.stringify(value)}`,
  };
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
  doc.addIn(networksPath, doc.createNode(network));
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
  doc.addIn(servicesPath, doc.createNode(service));
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
