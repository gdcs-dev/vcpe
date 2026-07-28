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

describe('applyMutation — renameService', () => {
  it('renames the service and updates dependsOn references', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'renameService', oldName: 'bng', newName: 'core-bng',
    });
    const parsed = parse(result);
    if ('error' in parsed) throw new Error(parsed.error);
    // old name gone, new name present
    expect(parsed.model.spec.services.find(s => s.name === 'bng')).toBeUndefined();
    expect(parsed.model.spec.services.find(s => s.name === 'core-bng')).toBeDefined();
    // gateway's dependsOn updated
    const gateway = parsed.model.spec.services.find(s => s.name === 'gateway');
    expect(gateway?.dependsOn ?? []).toContain('core-bng');
    expect(gateway?.dependsOn ?? []).not.toContain('bng');
  });

  it('is a no-op when new name equals old name', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'renameService', oldName: 'bng', newName: 'bng',
    });
    expect(result).toBe(MINIMAL_MANIFEST);
  });
});

describe('applyMutation — setScalar null handling', () => {
  it('deletes a key when value is null instead of writing YAML null', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'setScalar',
      path: ['spec', 'networks', 0, 'nat'],
      value: null,
    });
    expect(result).not.toContain('nat: null');
    expect(result).not.toMatch(/nat:\s*null/);
  });

  it('prunes empty pool object when both start and end are cleared', () => {
    // First insert a network with a pool
    const withPool = roundTrip(MINIMAL_MANIFEST, {
      kind: 'insertNetwork',
      network: { role: 'test-pool', ipv4: { cidr: '10.99.0.0/24', gateway: '10.99.0.1', pool: { start: '10.99.0.10', end: '10.99.0.250' } } },
    });
    const netIdx = parse(withPool);
    if ('error' in netIdx) throw new Error(netIdx.error);
    const idx = netIdx.model.spec.networks.findIndex(n => n.role === 'test-pool');

    // Clear both pool fields
    const cleared = roundTrip(withPool,
      { kind: 'setScalar', path: ['spec', 'networks', idx, 'ipv4', 'pool', 'start'], value: null },
      { kind: 'setScalar', path: ['spec', 'networks', idx, 'ipv4', 'pool', 'end'], value: null },
    );
    // pool key itself should be gone — not { start: null, end: null }
    expect(cleared).not.toContain('pool:');
    expect(cleared).not.toContain('null');
  });

  it('does not serialise null fields from insertNetwork', () => {
    const result = roundTrip(MINIMAL_MANIFEST, {
      kind: 'insertNetwork',
      network: { role: 'nullish', nat: undefined, firewall: undefined, ipv4: { cidr: '10.1.0.0/24' } },
    });
    expect(result).not.toContain('nat: null');
    expect(result).not.toContain('firewall: null');
  });
});
