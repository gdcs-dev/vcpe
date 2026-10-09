import { describe, it, expect } from 'vitest';
import { parse, validateWireless } from './parse';
import { readFileSync } from 'fs';
import { resolve } from 'path';

// Path to the actual example.yaml manifest in the repo
// __dirname = .../extensions/vcpe-visual-editor/webview/src/yaml/
// 5 levels up → repo root
const EXAMPLE_YAML_PATH = resolve(
  __dirname,
  '../../../../../manifests/example.yaml'
);

function loadExample(): string {
  return readFileSync(EXAMPLE_YAML_PATH, 'utf8');
}

describe('parse', () => {
  it('parses example.yaml to a valid ManifestModel', () => {
    const result = parse(loadExample());
    expect('error' in result).toBe(false);
    if ('error' in result) return;

    const { model } = result;
    expect(model.apiVersion).toBe('vcpe.dev/v1');
    expect(model.kind).toBe('Deployment');
    expect(model.metadata.name).toBe('example');
    expect(model.spec.networks.length).toBeGreaterThan(0);
    expect(model.spec.services.length).toBeGreaterThan(0);
  });

  it('returns a parsed doc AST alongside the model', () => {
    const result = parse(loadExample());
    expect('error' in result).toBe(false);
    if ('error' in result) return;
    expect(result.doc).toBeDefined();
    expect(typeof String(result.doc)).toBe('string');
  });

  it('returns ParseError for invalid YAML', () => {
    const result = parse(':::invalid:::');
    expect('error' in result).toBe(true);
  });

  it('returns ParseError for wrong apiVersion', () => {
    const result = parse('apiVersion: v1\nkind: Deployment\nmetadata:\n  name: x\nspec:\n  networks: []\n  services: []\n');
    expect('error' in result).toBe(true);
    if (!('error' in result)) return;
    expect(result.error).toContain('vcpe.dev/v1');
  });

  it('parses all network fields', () => {
    const result = parse(loadExample());
    if ('error' in result) throw new Error(result.error);
    const wan = result.model.spec.networks.find(n => n.role === 'wan');
    expect(wan).toBeDefined();
    expect(wan?.nat).toBe(true);
    expect(wan?.firewall).toBe(true);
    expect(wan?.ipv4?.cidr).toMatch(/\d+\.\d+\.\d+\.\d+\/\d+/);
  });

  it('parses all service fields including interfaces and dependsOn', () => {
    const result = parse(loadExample());
    if ('error' in result) throw new Error(result.error);
    const bng = result.model.spec.services.find(s => s.name === 'bng');
    expect(bng).toBeDefined();
    expect(bng?.type).toBe('bng');
    expect(bng?.image.repository).toBe('ghcr.io/gdcs-dev/bng');
    expect(bng?.interfaces?.length).toBeGreaterThan(0);

    const gateway = result.model.spec.services.find(s => s.name === 'gateway');
    expect(gateway?.dependsOn).toContain('bng');
  });

  it('parses tri-band media, mirrored profiles, and ordered VAP slots', () => {
    const yaml = readFileSync(resolve(__dirname, '../../../../../manifests/dev/wireless.yaml'), 'utf8');
    const result = parse(yaml);
    if ('error' in result) throw new Error(result.error);
    expect(result.model.spec.wirelessMedia?.map(medium => medium.band)).toEqual(['2.4ghz', '5ghz', '6ghz']);
    expect(result.model.spec.wirelessNetworks?.find(profile => profile.name === 'home')?.passphraseSecretRef).toBe('home-wifi');
    const gateway = result.model.spec.services.find(service => service.name === 'gateway');
    expect(gateway?.radios?.map(radio => radio.vaps?.length)).toEqual([8, 8, 8]);
    expect(gateway?.radios?.[2].vaps?.[1]).toEqual({slot: 1, network: 'guest', bridge: 'brguest'});
    expect(result.model.spec.services.find(service => service.name === 'client-6')?.radios?.[0].medium).toBe('rf6');
    expect(validateWireless(result.model)).toBeNull();
    result.model.spec.wirelessNetworks![0].security = 'open';
    expect(validateWireless(result.model)).toContain('6 GHz requires WPA3');
  });

  it('parses macvlan network with driverOptions', () => {
    const macvlanYaml = `
apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: test
spec:
  networks:
    - role: wan
      driver: macvlan
      driverOptions:
        parent: eth0
      ipamDriver: none
      ipv4:
        cidr: 10.0.0.0/24
  services: []
`;
    const result = parse(macvlanYaml);
    if ('error' in result) throw new Error(result.error);
    const wan = result.model.spec.networks[0];
    expect(wan.driver).toBe('macvlan');
    expect(wan.driverOptions?.parent).toBe('eth0');
  });
});
