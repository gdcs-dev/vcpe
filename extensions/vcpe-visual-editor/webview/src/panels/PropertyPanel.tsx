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

  return (
    <div>
      <Field label="Role" value={network.role} readOnly hint="Network role cannot be renamed in v1 — delete and recreate to change." />
      <Field label="Driver" value={network.driver ?? 'bridge (default)'} readOnly />
      {network.driverOptions?.parent && <Field label="Parent NIC" value={network.driverOptions.parent} readOnly />}
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
      <Field label="Name" value={service.name} readOnly hint="Rename not supported in v1 — would break dependsOn cross-references." />
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
      {(service.interfaces?.length ?? 0) > 0 && <>
        <Divider label="Interfaces" />
        {service.interfaces!.map((iface, j) => (
          <div key={iface.role} style={{ marginBottom: 10, paddingLeft: 6, borderLeft: '2px solid #333' }}>
            <div style={{ fontSize: 10, color: '#666', marginBottom: 4, fontWeight: 600 }}>{iface.role}</div>
            <Field label="Device name" value={iface.device ?? ''}
              onCommit={v => commit(['spec', 'services', idx, 'interfaces', j, 'device'], v || null)} />
            {iface.ipv4 && <Field label="IPv4" value={iface.ipv4} readOnly />}
            {iface.mac && <Field label="MAC" value={iface.mac} readOnly />}
          </div>
        ))}
      </>}
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
    </div>
  );
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
      {(model.spec.secrets?.length ?? 0) > 0 && <>
        <Divider label="Secrets" />
        {model.spec.secrets!.map(s => (
          <Field key={s.name} label={s.name} value={`${s.provider} / ${s.key}`} readOnly />
        ))}
      </>}
    </div>
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
