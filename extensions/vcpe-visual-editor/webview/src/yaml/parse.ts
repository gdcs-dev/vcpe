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
  role: string; device?: string; bridge?: string; mac?: string;
  ipv4?: string; ipv6?: string; defaultRoute?: boolean;
}
export interface BridgeSpec { name: string; ipv4?: string; ipv6?: string; dhcpStart?: string; dhcpEnd?: string; }
export interface WirelessMedium { name: string; band: string; channel: number; widthMHz: number }
export interface WirelessProfile { name: string; ssid: string; security: string; passphraseSecretRef?: string }
export interface VAP { slot: number; network: string; bridge: string }
export interface Radio {
  name: string; mode: string; medium: string; device: string;
  network?: string; vaps?: VAP[]; addressing?: string; defaultRoute?: boolean;
}
export interface SecretRef { name: string; provider: string; key: string }
export interface Service {
  name: string; type: string; replicas: number; image: Image;
  dependsOn?: string[]; interfaces?: Interface[];
  bridges?: BridgeSpec[];
  radios?: Radio[];
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
  wirelessMedia?: WirelessMedium[];
  wirelessNetworks?: WirelessProfile[];
  maxReplicasPerService?: number;
  maxActiveDeployments?: number;
}
export interface ManifestModel {
  apiVersion: string;
  kind: string;
  metadata: Metadata;
  spec: Spec;
}

export function validateWireless(model: ManifestModel): string | null {
  const media = model.spec.wirelessMedia ?? [];
  const profiles = model.spec.wirelessNetworks ?? [];
  if (new Set(media.map(item => item.name)).size !== media.length) return 'Wireless medium names must be unique';
  if (new Set(profiles.map(item => item.name)).size !== profiles.length) return 'Wireless profile names must be unique';
  for (const medium of media) {
    const channel = medium.channel;
    if (medium.widthMHz !== 20 || !(
      (medium.band === '2.4ghz' && channel >= 1 && channel <= 14) ||
      (medium.band === '5ghz' && [36, 40, 44, 48, 149, 153, 157, 161, 165].includes(channel)) ||
      (medium.band === '6ghz' && channel >= 5 && channel <= 229 && (channel - 5) % 16 === 0)
    )) return `Invalid band, channel, or width for ${medium.name}`;
  }
  const apPairs = new Map<string, number>();
  const stations: string[] = [];
  for (const service of model.spec.services) {
    for (const radio of service.radios ?? []) {
      const medium = media.find(item => item.name === radio.medium);
      if (!medium) return `Radio ${radio.name} references an unknown medium`;
      if (radio.mode === 'station') {
        const profile = profiles.find(item => item.name === radio.network);
        if (!profile) return `Station ${radio.name} references an unknown profile`;
        if (medium.band === '6ghz' && profile.security !== 'wpa3-personal') return `6 GHz requires WPA3-Personal: ${profile.name}`;
        stations.push(`${radio.medium}/${radio.network}`);
        continue;
      }
      if (radio.mode !== 'ap') return `Invalid radio mode: ${radio.name}`;
      const vaps = radio.vaps ?? [];
      if (vaps.length < 1 || vaps.length > 8 || !vaps.some(vap => vap.slot === 0)) return `Radio ${radio.name} needs 1-8 VAPs including slot 0`;
      if (new Set(vaps.map(vap => vap.slot)).size !== vaps.length || vaps.some(vap => vap.slot < 0 || vap.slot > 7)) return `Invalid or duplicate VAP slot on ${radio.name}`;
      if (new Set(vaps.map(vap => vap.network)).size !== vaps.length) return `Radio ${radio.name} repeats a profile`;
      for (const vap of vaps) {
        const profile = profiles.find(item => item.name === vap.network);
        if (!profile) return `VAP ${radio.name}/${vap.slot} references an unknown profile`;
        if (!service.bridges?.some(bridge => bridge.name === vap.bridge)) return `VAP ${radio.name}/${vap.slot} references an undeclared bridge`;
        if (medium.band === '6ghz' && profile.security !== 'wpa3-personal') return `6 GHz requires WPA3-Personal: ${profile.name}`;
        const pair = `${radio.medium}/${vap.network}`;
        apPairs.set(pair, (apPairs.get(pair) ?? 0) + 1);
      }
    }
  }
  if ([...apPairs.values()].some(count => count > 1)) return 'A medium/profile pair has multiple AP VAPs';
  if (stations.some(pair => apPairs.get(pair) !== 1)) return 'A station needs exactly one AP VAP on its medium/profile';
  return null;
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
  const mediaNode = specNode.get('wirelessMedia', true);
  const profilesNode = specNode.get('wirelessNetworks', true);

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
    wirelessMedia: isSeq(mediaNode) ? mediaNode.items.filter(isMap).map(node => ({
      name: getString(node, 'name') ?? '', band: getString(node, 'band') ?? '',
      channel: getNumber(node, 'channel') ?? 0, widthMHz: getNumber(node, 'widthMHz') ?? 0,
    })) : [],
    wirelessNetworks: isSeq(profilesNode) ? profilesNode.items.filter(isMap).map(node => ({
      name: getString(node, 'name') ?? '', ssid: getString(node, 'ssid') ?? '',
      security: getString(node, 'security') ?? '', passphraseSecretRef: getString(node, 'passphraseSecretRef'),
    })) : [],
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
  const bridgesNode = node.get('bridges', true);
  const radiosNode = node.get('radios', true);
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
    bridges: isSeq(bridgesNode) ? bridgesNode.items.filter(isMap).map(parseBridgeSpec) : [],
    radios: isSeq(radiosNode) ? radiosNode.items.filter(isMap).map(parseRadio) : [],
    ports: isSeq(portsNode) ? portsNode.items.map(s => String(isScalar(s) ? s.value : s)) : [],
    volumes: isSeq(volsNode) ? volsNode.items.map(s => String(isScalar(s) ? s.value : s)) : [],
    config: node.get('config'),
  };
}

function parseRadio(radio: YAMLMap): Radio {
  const vapsNode = radio.get('vaps', true);
  return {
      name: getString(radio, 'name') ?? '', mode: getString(radio, 'mode') ?? '',
      medium: getString(radio, 'medium') ?? '', device: getString(radio, 'device') ?? '',
      network: getString(radio, 'network'), addressing: getString(radio, 'addressing'),
      defaultRoute: getBoolean(radio, 'defaultRoute'),
      vaps: isSeq(vapsNode)
        ? vapsNode.items.filter(isMap).map(vap => ({
          slot: getNumber(vap, 'slot') ?? 0, network: getString(vap, 'network') ?? '',
          bridge: getString(vap, 'bridge') ?? '',
        })) : [],
  };
}

function parseBridgeSpec(node: YAMLMap): BridgeSpec {
  return {
    name: getString(node, 'name') ?? '',
    ipv4: getString(node, 'ipv4'),
    ipv6: getString(node, 'ipv6'),
    dhcpStart: getString(node, 'dhcpStart'),
    dhcpEnd: getString(node, 'dhcpEnd'),
  };
}

function parseInterface(node: YAMLMap): Interface {
  return {
    role: getString(node, 'role') ?? '',
    device: getString(node, 'device'),
    bridge: getString(node, 'bridge'),
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
