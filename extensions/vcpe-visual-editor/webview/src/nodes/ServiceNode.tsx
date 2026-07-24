import React from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { roleColor } from '../utils/roleColor';

export interface NetworkChip {
  role: string;
  device?: string;
  ipv4?: string;
  defaultRoute?: boolean;
}

export interface ServiceNodeData {
  name: string;
  type: string;
  replicas: number;
  networks: NetworkChip[];
}

/**
 * ServiceNode renders a service card. Each network interface is a row with its
 * own React Flow Handle so edges connect to the specific network chip.
 * Handle ID: `iface-{role}` — used by App.tsx when generating edges.
 * Label: device name if set; 'eth{i}' for generic-container; role name otherwise.
 */
export function ServiceNode({ data, selected }: NodeProps<ServiceNodeData>) {
  // Infrastructure services (bng, gateway) show meaningful role names on their
  // interfaces (wan, cm, lan-p1…). All other service types are treated as
  // clients and show generic eth{i} names so their connections can be freely
  // moved to any available network.
  const isGeneric = !['bng', 'gateway'].includes(data.type);

  return (
    <div
      style={{
        minWidth: 180,
        borderRadius: 6,
        border: `1.5px solid ${selected ? '#4E9AF1' : '#444'}`,
        background: 'var(--vscode-editor-background, #1e1e1e)',
        boxShadow: selected ? '0 0 0 2px #4E9AF144' : undefined,
        userSelect: 'none',
        position: 'relative',
      }}
    >
      {/* dependsOn handles — top of node */}
      <Handle type="target" position={Position.Top} id="dep-target"
        style={{ width: 8, height: 8, background: '#666', left: '35%' }} />
      <Handle type="source" position={Position.Top} id="dep-source"
        style={{ width: 8, height: 8, background: '#666', left: '65%' }} />

      {/* Header */}
      <div style={{ padding: '6px 10px', borderBottom: data.networks.length ? '1px solid #2a2a2a' : undefined }}>
        <div style={{ fontWeight: 700, fontSize: 13 }}>{data.name}</div>
        <div style={{ fontSize: 10, color: '#888' }}>{data.type} · ×{data.replicas}</div>
      </div>

      {/* One row per network interface — each row has its own Handle */}
      {data.networks.map((n, i) => {
        const color = roleColor(n.role);
        const label = n.device || (isGeneric ? `eth${i}` : n.role);
        const isLast = i === data.networks.length - 1;
        return (
          <div
            key={n.role}
            style={{
              position: 'relative',
              display: 'flex',
              alignItems: 'center',
              padding: '4px 28px 4px 10px', // right padding for the handle
              borderBottom: isLast ? undefined : '1px solid #2a2a2a',
              gap: 6,
            }}
          >
            {/* The Handle is positioned at the right edge of this row */}
            <Handle
              type="source"
              position={Position.Right}
              id={`iface-${n.role}`}
              style={{ right: 4, width: 10, height: 10, background: color, border: `2px solid ${color}88` }}
            />
            {/* Colored dot */}
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: color, flexShrink: 0 }} />
            {/* Interface label */}
            <span style={{ fontSize: 11, color, fontWeight: 500 }}>{label}</span>
            {n.ipv4 && (
              <span style={{ fontSize: 10, color: '#777', marginLeft: 2 }}>{n.ipv4}</span>
            )}
            {n.defaultRoute && (
              <span style={{ fontSize: 10, color: '#888', marginLeft: 'auto' }}>↑ default</span>
            )}
          </div>
        );
      })}
    </div>
  );
}
