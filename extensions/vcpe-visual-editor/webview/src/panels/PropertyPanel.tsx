import React, { useState, useEffect } from 'react';
import type { ManifestModel, Network, Service } from '../yaml/parse';
import { applyMutation } from '../yaml/serialize';

interface Props {
  model: ManifestModel | null;
  selectedNodeId: string | null;
  onMutation: (description: string, newYaml: string) => void;
  rawYaml: string;
}

/**
 * PropertyPanel routes to the correct form based on selectedNodeId prefix.
 * Falls back to DeploymentSettingsDrawer when nothing is selected.
 */
export function PropertyPanel({ model, selectedNodeId, onMutation, rawYaml }: Props) {
  if (!model) return null;

  const content = (() => {
    if (!selectedNodeId) return <DeploymentSettingsDrawer model={model} onMutation={onMutation} rawYaml={rawYaml} />;
    if (selectedNodeId.startsWith('network:')) {
      const role = selectedNodeId.slice('network:'.length);
      const net = model.spec.networks.find(n => n.role === role);
      if (net) return <NetworkForm model={model} network={net} onMutation={onMutation} rawYaml={rawYaml} />;
    }
    if (selectedNodeId.startsWith('service:')) {
      const name = selectedNodeId.slice('service:'.length);
      const svc = model.spec.services.find(s => s.name === name);
      if (svc) return <ServiceForm model={model} service={svc} onMutation={onMutation} rawYaml={rawYaml} />;
    }
    return <DeploymentSettingsDrawer model={model} onMutation={onMutation} rawYaml={rawYaml} />;
  })();

  return (
    <div style={styles.container}>
      <div style={styles.header}>
        {selectedNodeId
          ? selectedNodeId.startsWith('network:') ? '◼ Network' : '◻ Service'
          : 'Deployment Settings'}
      </div>
      <div style={styles.body}>{content}</div>
    </div>
  );
}

// ─── NetworkForm ──────────────────────────────────────────────────────────────

function NetworkForm({ model, network, onMutation, rawYaml }: { model: ManifestModel; network: Network; onMutation: Props['onMutation']; rawYaml: string }) {
  const idx = model.spec.networks.findIndex(n => n.role === network.role);

  const commit = (path: (string | number)[], value: unknown) => {
    const { newYaml, description } = applyMutation(rawYaml, { kind: 'setScalar', path, value });
    onMutation(description, newYaml);
  };

  const isMacvlan = network.driver === 'macvlan' || network.driver === 'ipvlan';

  return (
    <div>
      <Field label="Role" value={network.role} readOnly hint="Network role cannot be renamed in v1 — delete and recreate to change." />
      <Field label="Driver" value={network.driver ?? 'bridge (default)'} readOnly />
      {isMacvlan && (
        <Field label="Parent NIC" value={network.driverOptions?.parent ?? ''}
          hint="Host network interface to attach the macvlan/ipvlan to (e.g. eth0, ens3)"
          onCommit={v => commit(['spec', 'networks', idx, 'driverOptions', 'parent'], v || null)} />
      )}
      <Field label="IPAM Driver" value={network.ipamDriver ?? ''} readOnly />
      <CheckField label="NAT" checked={!!network.nat}
        onChange={v => commit(['spec', 'networks', idx, 'nat'], v)} />
      <CheckField label="Firewall" checked={!!network.firewall}
        onChange={v => commit(['spec', 'networks', idx, 'firewall'], v)} />
      {network.ipv4 && <>
        <Divider label="IPv4" />
        <Field label="CIDR" value={network.ipv4.cidr}
          onCommit={v => commit(['spec', 'networks', idx, 'ipv4', 'cidr'], v)} />
        <Field label="Gateway" value={network.ipv4.gateway ?? ''}
          onCommit={v => commit(['spec', 'networks', idx, 'ipv4', 'gateway'], v)} />
        {network.ipv4.pool && <>
          <Field label="Pool start" value={network.ipv4.pool.start}
            onCommit={v => commit(['spec', 'networks', idx, 'ipv4', 'pool', 'start'], v)} />
          <Field label="Pool end" value={network.ipv4.pool.end}
            onCommit={v => commit(['spec', 'networks', idx, 'ipv4', 'pool', 'end'], v)} />
        </>}
      </>}
    </div>
  );
}

// ─── ServiceForm ──────────────────────────────────────────────────────────────

function ServiceForm({ model, service, onMutation, rawYaml }: { model: ManifestModel; service: Service; onMutation: Props['onMutation']; rawYaml: string }) {
  const idx = model.spec.services.findIndex(s => s.name === service.name);

  const commit = (path: (string | number)[], value: unknown) => {
    const { newYaml, description } = applyMutation(rawYaml, { kind: 'setScalar', path, value });
    onMutation(description, newYaml);
  };

  // Local state for the config textarea so edits don't round-trip on every keystroke
  const [configText, setConfigText] = useState('');
  useEffect(() => {
    try {
      setConfigText(service.config ? JSON.stringify(service.config, null, 2) : '');
    } catch {
      setConfigText('');
    }
  }, [service.name, service.config]);

  const commitConfig = () => {
    if (!configText.trim()) return;
    try {
      const { newYaml, description } = applyMutation(rawYaml, {
        kind: 'setConfig',
        serviceIndex: idx,
        configYaml: configText,
      });
      onMutation(description, newYaml);
    } catch {
      // invalid YAML — don't apply
    }
  };

  return (
    <div>
      <Field label="Name" value={service.name}
        hint="Renaming updates all dependsOn references automatically."
        onCommit={v => {
          const trimmed = v.trim();
          if (!trimmed || trimmed === service.name) return;
          const { newYaml, description } = applyMutation(rawYaml, {
            kind: 'renameService', oldName: service.name, newName: trimmed,
          });
          onMutation(description, newYaml);
        }} />
      <Field label="Type" value={service.type} readOnly />
      <Field label="Replicas" value={String(service.replicas)} type="number"
        onCommit={v => commit(['spec', 'services', idx, 'replicas'], Math.max(1, Number(v)))} />
      <Divider label="Image" />
      <Field label="Repository" value={service.image.repository}
        onCommit={v => commit(['spec', 'services', idx, 'image', 'repository'], v)} />
      <Field label="Tag" value={service.image.tag ?? ''}
        onCommit={v => commit(['spec', 'services', idx, 'image', 'tag'], v || null)} />
      <Field label="Pull Policy" value={service.image.pullPolicy ?? ''}
        onCommit={v => commit(['spec', 'services', idx, 'image', 'pullPolicy'], v || null)} />
      {(service.dependsOn?.length ?? 0) > 0 && <>
        <Divider label="Depends On" />
        {service.dependsOn!.map(d => <Field key={d} label="" value={d} readOnly />)}
      </>}
      <Divider label="Interfaces" />
      {service.interfaces?.map((iface, j) => {
        const bridgeNames = (service.bridges ?? []).map(b => b.name);
        return (
        <div key={iface.role} style={{ marginBottom: 8, paddingLeft: 6, borderLeft: '2px solid #333', position: 'relative' }}>
          {/* Remove button */}
          <button
            onClick={() => {
              const { newYaml, description } = applyMutation(rawYaml, {
                kind: 'removeInterface', serviceIndex: idx, ifaceIndex: j,
              });
              onMutation(description, newYaml);
            }}
            style={{ position: 'absolute', top: 0, right: 0, background: 'none', border: 'none', color: '#666', cursor: 'pointer', fontSize: 14, padding: '0 2px', lineHeight: 1 }}
            title="Remove interface">×</button>
          <div style={{ fontSize: 10, color: '#666', marginBottom: 4, fontWeight: 600, paddingRight: 14 }}>{iface.role}</div>
          <Field label="Device name" value={iface.device ?? ''}
            onCommit={v => commit(['spec', 'services', idx, 'interfaces', j, 'device'], v || null)} />
          {bridgeNames.length > 0 && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 4, marginBottom: 4 }}>
              <span style={{ fontSize: 10, color: '#666', minWidth: 72 }}>Bridge</span>
              <select
                value={iface.bridge ?? ''}
                onChange={e => commit(['spec', 'services', idx, 'interfaces', j, 'bridge'], e.target.value || null)}
                style={{ flex: 1, padding: '2px 4px', background: '#1e1e1e', color: '#ccc', border: '1px solid #444', borderRadius: 3, fontSize: 11 }}>
                <option value="">— none —</option>
                {bridgeNames.map(b => <option key={b} value={b}>{b}</option>)}
              </select>
            </div>
          )}
          {iface.ipv4 && <Field label="IPv4" value={iface.ipv4} readOnly />}
        </div>
        );
      })}
      {/* Add interface: dropdown of available network roles */}
      {(() => {
        const usedRoles = new Set(service.interfaces?.map(i => i.role) ?? []);
        const available = model.spec.networks.filter(n => !usedRoles.has(n.role));
        if (available.length === 0) return null;
        return (
          <select
            defaultValue=""
            onChange={e => {
              const role = e.target.value;
              if (!role) return;
              e.target.value = '';
              const { newYaml, description } = applyMutation(rawYaml, {
                kind: 'addInterface', serviceIndex: idx, iface: { role },
              });
              onMutation(description, newYaml);
            }}
            style={{ width: '100%', padding: '3px 6px', background: '#1e1e1e', color: '#777', border: '1px dashed #444', borderRadius: 3, fontSize: 11, marginBottom: 8 }}>
            <option value="" disabled>+ connect to network…</option>
            {available.map(n => (
              <option key={n.role} value={n.role}>{n.role}{n.ipv4?.cidr ? ` (${n.ipv4.cidr})` : ''}</option>
            ))}
          </select>
        );
      })()}
      {configText && <>
        <Divider label="Config (YAML)" />
        <textarea
          value={configText}
          onChange={e => setConfigText(e.target.value)}
          onBlur={commitConfig}
          spellCheck={false}
          style={{ width: '100%', minHeight: 120, background: '#1a1a1a', color: '#ccc', border: '1px solid #444', borderRadius: 3, padding: 6, fontSize: 11, fontFamily: 'monospace', resize: 'vertical' }}
        />
        <div style={{ fontSize: 10, color: '#666', marginTop: 2 }}>Saved on blur</div>
      </>}
      <BridgesSection service={service} serviceIndex={idx} onMutation={onMutation} rawYaml={rawYaml} />
    </div>
  );
}

// ─── BridgesSection ───────────────────────────────────────────────────────────

function BridgesSection({ service, serviceIndex, onMutation, rawYaml }: {
  service: Service; serviceIndex: number;
  onMutation: Props['onMutation']; rawYaml: string;
}) {
  const [newName, setNewName] = useState('');

  const commit = (path: (string | number)[], value: unknown) => {
    const { newYaml, description } = applyMutation(rawYaml, { kind: 'setScalar', path, value });
    onMutation(description, newYaml);
  };

  const addBridge = () => {
    const name = newName.trim();
    if (!name) return;
    const { newYaml, description } = applyMutation(rawYaml, {
      kind: 'insertBridge', serviceIndex, bridge: { name },
    });
    onMutation(description, newYaml);
    setNewName('');
  };

  const bridges = service.bridges ?? [];

  return <>
    <Divider label="Bridges" />
    {bridges.map((b, i) => (
      <div key={b.name} style={{ marginBottom: 8, paddingLeft: 6, borderLeft: '2px solid #2a4a2a', position: 'relative' }}>
        <button
          onClick={() => {
            const { newYaml, description } = applyMutation(rawYaml, {
              kind: 'deleteBridge', serviceIndex, bridgeIndex: i,
            });
            onMutation(description, newYaml);
          }}
          style={{ position: 'absolute', top: 0, right: 0, background: 'none', border: 'none', color: '#666', cursor: 'pointer', fontSize: 14, padding: '0 2px', lineHeight: 1 }}
          title="Remove bridge">×</button>
        <div style={{ fontSize: 10, color: '#4a9a4a', marginBottom: 4, fontWeight: 600, paddingRight: 14 }}>{b.name}</div>
        <Field label="IPv4 (CIDR)" value={b.ipv4 ?? ''}
          onCommit={v => commit(['spec', 'services', serviceIndex, 'bridges', i, 'ipv4'], v || null)} />
        <Field label="IPv6 (CIDR)" value={b.ipv6 ?? ''}
          onCommit={v => commit(['spec', 'services', serviceIndex, 'bridges', i, 'ipv6'], v || null)} />
        <Field label="DHCP start" value={b.dhcpStart ?? ''}
          onCommit={v => commit(['spec', 'services', serviceIndex, 'bridges', i, 'dhcpStart'], v || null)} />
        <Field label="DHCP end" value={b.dhcpEnd ?? ''}
          onCommit={v => commit(['spec', 'services', serviceIndex, 'bridges', i, 'dhcpEnd'], v || null)} />
      </div>
    ))}
    <div style={{ display: 'flex', gap: 4, marginBottom: 8 }}>
      <input
        value={newName}
        onChange={e => setNewName(e.target.value)}
        onKeyDown={e => { if (e.key === 'Enter') addBridge(); }}
        placeholder="bridge name…"
        style={{ flex: 1, padding: '3px 6px', background: '#1e1e1e', color: '#ccc', border: '1px dashed #444', borderRadius: 3, fontSize: 11 }}
      />
      <button
        onClick={addBridge}
        disabled={!newName.trim()}
        style={{ padding: '3px 8px', background: '#1e1e1e', color: newName.trim() ? '#4a9a4a' : '#555', border: '1px dashed #444', borderRadius: 3, fontSize: 11, cursor: newName.trim() ? 'pointer' : 'default' }}>
        + add
      </button>
    </div>
  </>;
}

// ─── DeploymentSettingsDrawer ────────────────────────────────────────────────

function DeploymentSettingsDrawer({ model, onMutation, rawYaml }: { model: ManifestModel; onMutation?: Props['onMutation']; rawYaml?: string }) {
  const commit = (path: (string | number)[], value: unknown) => {
    if (!onMutation || !rawYaml) return;
    const { newYaml, description } = applyMutation(rawYaml, { kind: 'setScalar', path, value });
    onMutation(description, newYaml);
  };

  return (
    <div>
      <Field label="Name" value={model.metadata.name}
        onCommit={v => commit(['metadata', 'name'], v)} />
      {model.spec.maxReplicasPerService !== undefined && (
        <Field label="Max replicas/svc" value={String(model.spec.maxReplicasPerService)} type="number"
          onCommit={v => commit(['spec', 'maxReplicasPerService'], Number(v))} />
      )}
      {model.spec.maxActiveDeployments !== undefined && (
        <Field label="Max active depl." value={String(model.spec.maxActiveDeployments)} type="number"
          onCommit={v => commit(['spec', 'maxActiveDeployments'], Number(v))} />
      )}
      {model.metadata.labels && Object.keys(model.metadata.labels).length > 0 && <>
        <Divider label="Labels" />
        {Object.entries(model.metadata.labels).map(([k, v]) => (
          <Field key={k} label={k} value={v} readOnly />
        ))}
      </>}
      {onMutation && rawYaml && (
        <NetworksSection model={model} onMutation={onMutation} rawYaml={rawYaml} />
      )}
      {(model.spec.secrets?.length ?? 0) > 0 && <>
        <Divider label="Secrets" />
        {model.spec.secrets!.map(s => (
          <Field key={s.name} label={s.name} value={`${s.provider} / ${s.key}`} readOnly />
        ))}
      </>}
    </div>
  );
}

// ─── NetworksSection ──────────────────────────────────────────────────────────

function NetworksSection({ model, onMutation, rawYaml }: { model: ManifestModel; onMutation: Props['onMutation']; rawYaml: string }) {
  const [newRole, setNewRole] = useState('');
  const [expanded, setExpanded] = useState<string | null>(null);

  const addNetwork = () => {
    const role = newRole.trim();
    if (!role) return;
    if (model.spec.networks.some(n => n.role === role)) return;
    const { newYaml, description } = applyMutation(rawYaml, {
      kind: 'insertNetwork',
      network: { role },
    });
    onMutation(description, newYaml);
    setNewRole('');
  };

  const deleteNetwork = (role: string) => {
    const { newYaml, description } = applyMutation(rawYaml, {
      kind: 'deleteNetwork',
      role,
    });
    onMutation(description, newYaml);
  };

  const commitNet = (idx: number, path: (string | number)[], value: unknown) => {
    const { newYaml, description } = applyMutation(rawYaml, {
      kind: 'setScalar',
      path: ['spec', 'networks', idx, ...path],
      value,
    });
    onMutation(description, newYaml);
  };

  return (
    <>
      <Divider label="Networks" />
      {model.spec.networks.map((net, idx) => {
        const isOpen = expanded === net.role;
        return (
          <div key={net.role} style={{ marginBottom: 6 }}>
            {/* Row header */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <button onClick={() => setExpanded(isOpen ? null : net.role)}
                style={{ flex: 1, textAlign: 'left', background: '#2a2a2a', border: '1px solid #444', borderRadius: 3, padding: '3px 6px', color: '#ccc', fontSize: 11, cursor: 'pointer' }}>
                {isOpen ? '▾' : '▸'} {net.role}
                {net.ipv4?.cidr && <span style={{ color: '#666', marginLeft: 6, fontSize: 10 }}>{net.ipv4.cidr}</span>}
                {net.nat && <span style={{ color: '#E67E22', marginLeft: 4, fontSize: 9 }}>nat</span>}
                {net.firewall && <span style={{ color: '#E74C3C', marginLeft: 4, fontSize: 9 }}>fw</span>}
              </button>
              <button onClick={() => deleteNetwork(net.role)}
                style={{ background: 'none', border: 'none', color: '#666', cursor: 'pointer', fontSize: 14, padding: '0 4px' }}
                title="Delete network">×</button>
            </div>
            {/* Expanded form */}
            {isOpen && (
              <div style={{ marginTop: 4, paddingLeft: 8, borderLeft: '2px solid #333' }}>
                <Field label="Role" value={net.role} readOnly hint="Role is immutable after creation" />
                <Field label="IPv4 CIDR" value={net.ipv4?.cidr ?? ''}
                  onCommit={v => commitNet(idx, ['ipv4', 'cidr'], v || null)} />
                <Field label="IPv4 Gateway" value={net.ipv4?.gateway ?? ''}
                  onCommit={v => commitNet(idx, ['ipv4', 'gateway'], v || null)} />
                <Field label="Pool Start" value={net.ipv4?.pool?.start ?? ''}
                  onCommit={v => commitNet(idx, ['ipv4', 'pool', 'start'], v || null)} />
                <Field label="Pool End" value={net.ipv4?.pool?.end ?? ''}
                  onCommit={v => commitNet(idx, ['ipv4', 'pool', 'end'], v || null)} />
                <CheckField label="NAT" checked={!!net.nat}
                  onChange={v => commitNet(idx, ['nat'], v || null)} />
                <CheckField label="Firewall" checked={!!net.firewall}
                  onChange={v => commitNet(idx, ['firewall'], v || null)} />
                <Field label="Driver" value={net.driver ?? ''}
                  onCommit={v => commitNet(idx, ['driver'], v || null)} />
                <Field label="IPAM Driver" value={net.ipamDriver ?? ''}
                  onCommit={v => commitNet(idx, ['ipamDriver'], v || null)} />
                {(net.driver === 'macvlan' || net.driver === 'ipvlan') && (
                  <Field label="Parent NIC" value={net.driverOptions?.parent ?? ''}
                    hint="Host NIC for the macvlan/ipvlan (e.g. eth0, ens3)"
                    onCommit={v => commitNet(idx, ['driverOptions', 'parent'], v || null)} />
                )}
              </div>
            )}
          </div>
        );
      })}
      {/* Add new network */}
      <div style={{ display: 'flex', gap: 4, marginTop: 4 }}>
        <input
          value={newRole}
          onChange={e => setNewRole(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && addNetwork()}
          placeholder="role name…"
          style={{ flex: 1, padding: '3px 6px', background: '#1e1e1e', color: '#ddd', border: '1px solid #555', borderRadius: 3, fontSize: 11 }}
        />
        <button onClick={addNetwork}
          style={{ padding: '3px 8px', background: '#4E9AF133', color: '#4E9AF1', border: '1px solid #4E9AF155', borderRadius: 3, cursor: 'pointer', fontSize: 11 }}>
          + Add
        </button>
      </div>
    </>
  );
}

// ─── Shared form components ────────────────────────────────────────────────

function Field({
  label, value, readOnly = false, hint, type = 'text', onCommit,
}: {
  label: string;
  value: string;
  readOnly?: boolean;
  hint?: string;
  type?: string;
  onCommit?: (value: string) => void;
}) {
  const [local, setLocal] = useState(value);
  // Sync when prop changes (e.g. canvas re-render after external YAML edit)
  useEffect(() => { setLocal(value); }, [value]);

  return (
    <div style={{ marginBottom: 8 }}>
      {label && (
        <div style={{ fontSize: 10, color: '#888', marginBottom: 2 }}>
          {label}
          {hint && <span title={hint} style={{ marginLeft: 4, cursor: 'help' }}>ⓘ</span>}
          {!readOnly && <span style={{ marginLeft: 4, color: '#4E9AF155', fontSize: 9 }}>editable</span>}
        </div>
      )}
      <input
        type={type}
        readOnly={readOnly}
        value={local}
        onChange={e => !readOnly && setLocal(e.target.value)}
        onBlur={() => {
          if (!readOnly && onCommit && local !== value) {
            onCommit(local);
          }
        }}
        onKeyDown={e => {
          if (e.key === 'Enter' && !readOnly && onCommit && local !== value) {
            onCommit(local);
            (e.target as HTMLInputElement).blur();
          }
        }}
        style={{
          width: '100%', padding: '3px 6px',
          background: readOnly ? '#111' : '#1e1e1e',
          color: readOnly ? '#888' : '#ddd',
          border: `1px solid ${readOnly ? '#333' : '#555'}`,
          borderRadius: 3, fontSize: 12, fontFamily: 'inherit',
          cursor: readOnly ? 'default' : 'text',
        }}
      />
    </div>
  );
}

function CheckField({ label, checked, onChange }: { label: string; checked: boolean; onChange?: (v: boolean) => void }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
      <input
        type="checkbox"
        checked={checked}
        readOnly={!onChange}
        style={{ cursor: onChange ? 'pointer' : 'default' }}
        onChange={e => onChange?.(e.target.checked)}
      />
      <span style={{ fontSize: 12 }}>{label}</span>
    </div>
  );
}

function Divider({ label }: { label: string }) {
  return (
    <div style={{ fontSize: 10, fontWeight: 700, color: '#666', textTransform: 'uppercase', borderBottom: '1px solid #333', padding: '8px 0 3px', marginBottom: 6, letterSpacing: 0.5 }}>
      {label}
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  container: { width: 220, borderLeft: '1px solid #333', background: 'var(--vscode-sideBar-background, #252526)', display: 'flex', flexDirection: 'column', overflowY: 'auto' },
  header: { fontWeight: 700, fontSize: 11, color: '#888', textTransform: 'uppercase', padding: '8px 12px 4px', letterSpacing: 0.5, borderBottom: '1px solid #333' },
  body: { padding: 12, overflowY: 'auto', flex: 1 },
};
