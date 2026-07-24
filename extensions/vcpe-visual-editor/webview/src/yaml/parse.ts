import { parseDocument, Document, YAMLMap, YAMLSeq, Scalar, isMap, isSeq, isScalar } from 'yaml';

// ─── ManifestModel ────────────────────────────────────────────────────────────

export interface Pool { start: string; end: string }
export interface AddressFamily { cidr: string; gateway?: string; pool?: Pool }
export interface Network {
  role: string; bridge?: string; nat?: boolean; firewall?: boolean;
  ipv4?: AddressFamily; ipv6?: AddressFamily;
  driver?: string; driverOptions?: Record<string, string>; ipamDriver?: string;
}
export interface Image {
  repository: string; tag?: string; buildContext?: string;
  containerfile?: string; pullPolicy?: string;
}
export interface Interface {
  role: string; device?: string; mac?: string;
  ipv4?: string; ipv6?: string; defaultRoute?: boolean;
}
export interface SecretRef { name: string; provider: string; key: string }
export interface Service {
  name: string; type: string; replicas: number; image: Image;
  dependsOn?: string[]; interfaces?: Interface[];
  ports?: string[]; volumes?: string[];
  config?: unknown;
}
export interface Metadata {
  name: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}
export interface Spec {
  networks: Network[];
  services: Service[];
  secrets?: SecretRef[];
  maxReplicasPerService?: number;
  maxActiveDeployments?: number;
}
export interface ManifestModel {
  apiVersion: string;
  kind: string;
  metadata: Metadata;
  spec: Spec;
}

// ─── Parser ───────────────────────────────────────────────────────────────────

export interface ParseResult {
  model: ManifestModel;
  doc: Document;          // preserved AST for surgical mutations
}

export interface ParseError {
  error: string;
  line?: number;
}

/**
 * parse converts YAML text to a ManifestModel plus the raw yaml.Document AST.
 * Returns a ParseError if the YAML is syntactically invalid or not a vcpe.dev/v1 manifest.
 */
export function parse(yamlText: string): ParseResult | ParseError {
  let doc: Document;
  try {
    doc = parseDocument(yamlText, { strict: false });
  } catch (e) {
    return { error: String(e) };
  }

  if (doc.errors && doc.errors.length > 0) {
    const first = doc.errors[0];
    return {
      error: first.message,
      line: first.linePos?.[0]?.line,
    };
  }

  const root = doc.contents;
  if (!isMap(root)) {
    return { error: 'Manifest must be a YAML mapping at the top level' };
  }

  const apiVersion = getString(root, 'apiVersion');
  if (apiVersion !== 'vcpe.dev/v1') {
    return { error: `Unsupported apiVersion: ${apiVersion ?? '(missing)'}. Expected "vcpe.dev/v1"` };
  }

  const kind = getString(root, 'kind');
  if (kind !== 'Deployment') {
    return { error: `Unsupported kind: ${kind ?? '(missing)'}. Expected "Deployment"` };
  }

  const metaNode = root.get('metadata', true);
  const specNode = root.get('spec', true);

  if (!isMap(metaNode)) return { error: 'metadata must be a mapping' };
  if (!isMap(specNode)) return { error: 'spec must be a mapping' };

  const metadata: Metadata = {
    name: getString(metaNode, 'name') ?? '',
    labels: getStringMap(metaNode, 'labels'),
    annotations: getStringMap(metaNode, 'annotations'),
  };

  const networksNode = specNode.get('networks', true);
  const servicesNode = specNode.get('services', true);
  const secretsNode = specNode.get('secrets', true);

  const networks: Network[] = isSeq(networksNode)
    ? networksNode.items.filter(isMap).map(parseNetwork)
    : [];
  const services: Service[] = isSeq(servicesNode)
    ? servicesNode.items.filter(isMap).map(parseService)
    : [];
  const secrets: SecretRef[] = isSeq(secretsNode)
    ? secretsNode.items.filter(isMap).map(parseSecretRef)
    : [];

  const spec: Spec = {
    networks,
    services,
    ...(secrets.length > 0 ? { secrets } : {}),
    maxReplicasPerService: getNumber(specNode, 'maxReplicasPerService'),
    maxActiveDeployments: getNumber(specNode, 'maxActiveDeployments'),
  };

  return {
    model: { apiVersion, kind, metadata, spec },
    doc,
  };
}

// ─── Node parsers ─────────────────────────────────────────────────────────────

function parseNetwork(node: YAMLMap): Network {
  return {
    role: getString(node, 'role') ?? '',
    bridge: getString(node, 'bridge'),
    nat: getBoolean(node, 'nat'),
    firewall: getBoolean(node, 'firewall'),
    ipv4: parseAddressFamily(node.get('ipv4', true) as YAMLMap | undefined),
    ipv6: parseAddressFamily(node.get('ipv6', true) as YAMLMap | undefined),
    driver: getString(node, 'driver'),
    driverOptions: getStringMap(node, 'driverOptions'),
    ipamDriver: getString(node, 'ipamDriver'),
  };
}

function parseAddressFamily(node: YAMLMap | undefined): AddressFamily | undefined {
  if (!isMap(node)) return undefined;
  const poolNode = node.get('pool', true) as YAMLMap | undefined;
  return {
    cidr: getString(node, 'cidr') ?? '',
    gateway: getString(node, 'gateway'),
    pool: isMap(poolNode)
      ? { start: getString(poolNode, 'start') ?? '', end: getString(poolNode, 'end') ?? '' }
      : undefined,
  };
}

function parseService(node: YAMLMap): Service {
  const imageNode = node.get('image', true) as YAMLMap | undefined;
  const ifacesNode = node.get('interfaces', true);
  const depsNode = node.get('dependsOn', true);
  const portsNode = node.get('ports', true);
  const volsNode = node.get('volumes', true);

  return {
    name: getString(node, 'name') ?? '',
    type: getString(node, 'type') ?? '',
    replicas: getNumber(node, 'replicas') ?? 1,
    image: isMap(imageNode) ? {
      repository: getString(imageNode, 'repository') ?? '',
      tag: getString(imageNode, 'tag'),
      buildContext: getString(imageNode, 'buildContext'),
      containerfile: getString(imageNode, 'containerfile'),
      pullPolicy: getString(imageNode, 'pullPolicy'),
    } : { repository: '' },
    dependsOn: isSeq(depsNode) ? depsNode.items.map(s => String(isScalar(s) ? s.value : s)) : [],
    interfaces: isSeq(ifacesNode) ? ifacesNode.items.filter(isMap).map(parseInterface) : [],
    ports: isSeq(portsNode) ? portsNode.items.map(s => String(isScalar(s) ? s.value : s)) : [],
    volumes: isSeq(volsNode) ? volsNode.items.map(s => String(isScalar(s) ? s.value : s)) : [],
    config: node.get('config'),
  };
}

function parseInterface(node: YAMLMap): Interface {
  return {
    role: getString(node, 'role') ?? '',
    device: getString(node, 'device'),
    mac: getString(node, 'mac'),
    ipv4: getString(node, 'ipv4'),
    ipv6: getString(node, 'ipv6'),
    defaultRoute: getBoolean(node, 'defaultRoute'),
  };
}

function parseSecretRef(node: YAMLMap): SecretRef {
  return {
    name: getString(node, 'name') ?? '',
    provider: getString(node, 'provider') ?? '',
    key: getString(node, 'key') ?? '',
  };
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getString(node: YAMLMap, key: string): string | undefined {
  const val = node.get(key);
  return typeof val === 'string' ? val : undefined;
}

function getNumber(node: YAMLMap, key: string): number | undefined {
  const val = node.get(key);
  return typeof val === 'number' ? val : undefined;
}

function getBoolean(node: YAMLMap, key: string): boolean | undefined {
  const val = node.get(key);
  return typeof val === 'boolean' ? val : undefined;
}

function getStringMap(node: YAMLMap, key: string): Record<string, string> | undefined {
  const sub = node.get(key, true);
  if (!isMap(sub)) return undefined;
  const result: Record<string, string> = {};
  for (const pair of sub.items) {
    const k = isScalar(pair.key) ? String(pair.key.value) : String(pair.key);
    const v = isScalar(pair.value) ? String(pair.value.value) : String(pair.value);
    result[k] = v;
  }
  return result;
}
