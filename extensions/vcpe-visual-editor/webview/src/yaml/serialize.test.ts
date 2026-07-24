import { describe, it, expect } from 'vitest';
import { applyMutation } from './serialize';
import { parse } from './parse';

const MINIMAL_MANIFEST = `apiVersion: vcpe.dev/v1
kind: Deployment
metadata:
  name: test
spec:
  networks:
    - role: mgmt
      ipv4:
        cidr: 10.0.0.0/24
    - role: wan
      nat: true
      ipv4:
        cidr: 10.1.0.0/24
  services:
    - name: bng
      type: bng
      replicas: 1
      image:
        repository: ghcr.io/gdcs-dev/bng
      interfaces:
        - role: mgmt
        - role: wan
    - name: gateway
      type: gateway
      replicas: 1
      image:
        repository: ghcr.io/gdcs-dev/gateway
      dependsOn:
        - bng
      interfaces:
        - role: wan
`;

function roundTrip(yaml: string, ...mutations: Parameters<typeof applyMutation>[1][]): string {
  let current = yaml;
  for (const mut of mutations) {
    current = applyMutation(current, mut).newYaml;
  }
  return current;
}

describe('applyMutation — setScalar', () => {
  it('updates a scalar value and preserves surrounding content', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'setScalar',
      path: ['spec', 'services', 0, 'replicas'],
      value: 3,
    });
    const parsed = parse(result);
    expect('error' in parsed).toBe(false);
    if ('error' in parsed) return;
    expect(parsed.model.spec.services[0].replicas).toBe(3);
    // Comments and surrounding YAML should still be present
    expect(result).toContain('apiVersion: vcpe.dev/v1');
  });

  it('updates metadata.name', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'setScalar',
      path: ['metadata', 'name'],
      value: 'updated-name',
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    expect(parsed.model.metadata.name).toBe('updated-name');
  });
});

describe('applyMutation — insertNetwork', () => {
  it('appends a new network to spec.networks', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'insertNetwork',
      network: { role: 'lan-p1', ipv4: { cidr: '192.168.10.0/24' } },
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    const lan = parsed.model.spec.networks.find(n => n.role === 'lan-p1');
    expect(lan).toBeDefined();
    expect(lan?.ipv4?.cidr).toBe('192.168.10.0/24');
  });
});

describe('applyMutation — insertService', () => {
  it('appends a new service to spec.services', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'insertService',
      service: {
        name: 'webpa', type: 'webpa', replicas: 1,
        image: { repository: 'ghcr.io/gdcs-dev/webpa' },
        interfaces: [{ role: 'mgmt' }],
      },
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    const webpa = parsed.model.spec.services.find(s => s.name === 'webpa');
    expect(webpa).toBeDefined();
    expect(webpa?.type).toBe('webpa');
  });
});

describe('applyMutation — addInterface', () => {
  it('adds an interface to a service', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'addInterface',
      serviceIndex: 1,
      iface: { role: 'mgmt', defaultRoute: false },
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    const ifaces = parsed.model.spec.services[1].interfaces ?? [];
    expect(ifaces.some(i => i.role === 'mgmt')).toBe(true);
  });
});

describe('applyMutation — removeInterface', () => {
  it('removes an interface from a service by index', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'removeInterface',
      serviceIndex: 0,
      ifaceIndex: 0,
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    const ifaces = parsed.model.spec.services[0].interfaces ?? [];
    expect(ifaces.every(i => i.role !== 'mgmt')).toBe(true);
  });
});

describe('applyMutation — deleteService (hard: cross-ref cleanup)', () => {
  it('removes the service and cleans dependsOn references', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'deleteService',
      name: 'bng',
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    // bng service is gone
    expect(parsed.model.spec.services.find(s => s.name === 'bng')).toBeUndefined();
    // gateway's dependsOn no longer references bng
    const gateway = parsed.model.spec.services.find(s => s.name === 'gateway');
    expect(gateway?.dependsOn ?? []).not.toContain('bng');
  });
});

describe('applyMutation — deleteNetwork (hard: interface cleanup)', () => {
  it('removes the network and cleans interface references in all services', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'deleteNetwork',
      role: 'wan',
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    // wan network is gone
    expect(parsed.model.spec.networks.find(n => n.role === 'wan')).toBeUndefined();
    // no service has an interface referencing wan
    for (const svc of parsed.model.spec.services) {
      for (const iface of svc.interfaces ?? []) {
        expect(iface.role).not.toBe('wan');
      }
    }
  });
});

describe('round-trip fidelity', () => {
  it('preserves apiVersion and kind after mutations', () => {
    const result = roundTrip(MINIMAL_MANIFEST,
      { kind: 'setScalar', path: ['metadata', 'name'], value: 'x' },
      { kind: 'insertNetwork', network: { role: 'cm' } },
    );
    expect(result).toContain('apiVersion: vcpe.dev/v1');
    expect(result).toContain('kind: Deployment');
  });
});
