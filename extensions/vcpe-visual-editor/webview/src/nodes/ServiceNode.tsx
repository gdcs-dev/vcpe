import React from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { roleColor } from '../utils/roleColor';

export interface NetworkChip {
  role: string;
  device?: string;
  bridge?: string;
  ipv4?: string;
  defaultRoute?: boolean;
}

export interface BridgeGroup {
  name: string;
  ipv4?: string;
  members: NetworkChip[];
}

export interface ServiceNodeData {
  name: string;
  type: string;
  replicas: number;
  networks: NetworkChip[];   // interfaces NOT assigned to a bridge
  bridges: BridgeGroup[];    // grouped bridge sections
}

/**
 * ServiceNode renders a service card.
 * - Non-bridged interfaces: one row per interface with a React Flow Handle (iface-{role})
 * - Bridged interfaces: grouped under a bridge header row with a bridge Handle (iface-bridge-{name})
 *   and member rows shown indented beneath it (no individual handles)
 */
export function ServiceNode({ data, selected }: NodeProps<ServiceNodeData>) {
  const isGeneric = !['bng', 'gateway'].includes(data.type);
  const hasBridges = data.bridges.length > 0;

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
      <div style={{ padding: '6px 10px', borderBottom: (data.networks.length || hasBridges) ? '1px solid #2a2a2a' : undefined }}>
        <div style={{ fontWeight: 700, fontSize: 13 }}>{data.name}</div>
        <div style={{ fontSize: 10, color: '#888' }}>{data.type} · ×{data.replicas}</div>
      </div>

      {/* Non-bridged interface rows — each has its own Handle */}
      {data.networks.map((n, i) => {
        const color = roleColor(n.role);
        const label = n.device || (isGeneric ? `eth${i}` : n.role);
        const isLast = i === data.networks.length - 1 && !hasBridges;
        return (
          <div key={n.role} style={{
            position: 'relative', display: 'flex', alignItems: 'center',
            padding: '4px 28px 4px 10px', borderBottom: isLast ? undefined : '1px solid #2a2a2a', gap: 6,
          }}>
            <Handle type="source" position={Position.Right} id={`iface-${n.role}`}
              style={{ right: 4, width: 10, height: 10, background: color, border: `2px solid ${color}88` }} />
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: color, flexShrink: 0 }} />
            <span style={{ fontSize: 11, color, fontWeight: 500 }}>{label}</span>
            {n.ipv4 && <span style={{ fontSize: 10, color: '#777', marginLeft: 2 }}>{n.ipv4}</span>}
            {n.defaultRoute && <span style={{ fontSize: 10, color: '#888', marginLeft: 'auto' }}>↑ default</span>}
          </div>
        );
      })}

      {/* Placeholder row shown when the service has no interfaces yet.
          Drag from another service's handle and drop here to add the interface. */}
      {data.networks.length === 0 && !hasBridges && (
        <div style={{
          position: 'relative', display: 'flex', alignItems: 'center',
          padding: '4px 28px 4px 10px', gap: 6,
        }}>
          <Handle type="target" position={Position.Right} id="iface-connect"
            style={{ right: 4, width: 10, height: 10, background: '#444', border: '1px dashed #777' }} />
          <span style={{ fontSize: 10, color: '#555', fontStyle: 'italic' }}>drag to connect</span>
        </div>
      )}

      {/* Bridge groups — header row has the Handle; member rows are indented */}
      {data.bridges.map((bg, bi) => {
        const bridgeColor = '#9B59B6';
        const isLastGroup = bi === data.bridges.length - 1;
        return (
          <React.Fragment key={`bridge-${bg.name}`}>
            {/* Bridge header row */}
            <div style={{
              position: 'relative', display: 'flex', alignItems: 'center',
              padding: '5px 28px 5px 10px', background: '#1a1a1a',
              borderTop: '1px solid #2a2a2a',
              borderBottom: bg.members.length ? '1px solid #222' : (isLastGroup ? undefined : '1px solid #2a2a2a'),
              gap: 6,
            }}>
              <Handle type="source" position={Position.Right} id={`iface-bridge-${bg.name}`}
                style={{ right: 4, width: 12, height: 12, background: bridgeColor, border: `2px solid ${bridgeColor}88`, borderRadius: 3 }} />
              <span style={{ fontSize: 10, color: bridgeColor }}>▣</span>
              <span style={{ fontSize: 12, color: bridgeColor, fontWeight: 700 }}>{bg.name}</span>
              {bg.ipv4 && <span style={{ fontSize: 10, color: '#777', marginLeft: 4 }}>{bg.ipv4}</span>}
            </div>
            {/* Bridge member rows — read-only, no individual handles */}
            {bg.members.map((m, mi) => {
              const mColor = roleColor(m.role);
              const mLabel = m.device || (isGeneric ? `eth${mi}` : m.role);
              const isLastMember = mi === bg.members.length - 1;
              return (
                <div key={m.role} style={{
                  display: 'flex', alignItems: 'center',
                  padding: '3px 10px 3px 22px',
                  borderBottom: (!isLastMember || !isLastGroup) ? '1px solid #222' : undefined,
                  gap: 5,
                }}>
                  <span style={{ width: 6, height: 6, borderRadius: '50%', background: mColor, flexShrink: 0 }} />
                  <span style={{ fontSize: 10, color: '#888' }}>{mLabel}</span>
                  <span style={{ fontSize: 9, color: '#555', marginLeft: 2 }}>{m.role}</span>
                  {m.ipv4 && <span style={{ fontSize: 9, color: '#555', marginLeft: 2 }}>{m.ipv4}</span>}
                </div>
              );
            })}
          </React.Fragment>
        );
      })}
    </div>
  );
}
