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

export type ServiceItem =
  | ({ kind: 'interface' } & NetworkChip)
  | { kind: 'bridge'; name: string; ipv4?: string; members: NetworkChip[] };

export interface ServiceNodeData {
  name: string;
  type: string;
  replicas: number;
  items: ServiceItem[];  // interfaces and bridge groups sorted together
}

/**
 * ServiceNode renders a service card.
 * - items are pre-sorted; each interface row has left+right handles
 * - bridge headers are cosmetic grouping elements (no handle); member rows have full handles
 */
export function ServiceNode({ data, selected }: NodeProps<ServiceNodeData>) {
  const isGeneric = !['bng', 'gateway'].includes(data.type);

  // Precompute display labels; eth{n} counter is shared across all rows for generic services.
  type Labeled<T> = T & { label: string };
  let ethCounter = 0;
  const labeled = data.items.map(item => {
    if (item.kind === 'interface') {
      return { ...item, label: item.device || (isGeneric ? `eth${ethCounter++}` : item.role) } as Labeled<typeof item>;
    }
    return {
      ...item,
      members: item.members.map(m => ({
        ...m,
        label: m.device || (isGeneric ? `eth${ethCounter++}` : m.role),
      })),
    };
  });

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
      <div style={{ padding: '6px 10px', borderBottom: data.items.length ? '1px solid #2a2a2a' : undefined }}>
        <div style={{ fontWeight: 700, fontSize: 13 }}>{data.name}</div>
        <div style={{ fontSize: 10, color: '#888' }}>{data.type} · ×{data.replicas}</div>
      </div>

      {/* Placeholder row shown when the service has no interfaces yet. */}
      {data.items.length === 0 && (
        <div style={{
          position: 'relative', display: 'flex', alignItems: 'center',
          padding: '4px 28px 4px 10px', gap: 6,
        }}>
          <Handle type="target" position={Position.Right} id="iface-connect"
            style={{ right: 4, width: 10, height: 10, background: '#444', border: '1px dashed #777' }} />
          <span style={{ fontSize: 10, color: '#555', fontStyle: 'italic' }}>drag to connect</span>
        </div>
      )}

      {/* Items — interfaces and bridge groups in pre-sorted order */}
      {labeled.map((item, itemIdx) => {
        const isLastItem = itemIdx === labeled.length - 1;

        if (item.kind === 'interface') {
          const color = roleColor(item.role);
          return (
            <div key={item.role} style={{
              position: 'relative', display: 'flex', alignItems: 'center',
              padding: '4px 28px 4px 28px',
              borderBottom: isLastItem ? undefined : '1px solid #2a2a2a',
              gap: 6,
            }}>
              <Handle type="source" position={Position.Right} id={`iface-${item.role}`}
                style={{ right: 4, width: 10, height: 10, background: color, border: `2px solid ${color}88` }} />
              <Handle type="source" position={Position.Left} id={`iface-${item.role}-left`}
                style={{ left: 4, width: 10, height: 10, background: color, border: `2px solid ${color}88` }} />
              <span style={{ width: 8, height: 8, borderRadius: '50%', background: color, flexShrink: 0 }} />
              <span style={{ fontSize: 11, color, fontWeight: 500 }}>{(item as { label: string }).label}</span>
              {item.ipv4 && <span style={{ fontSize: 10, color: '#777', marginLeft: 2 }}>{item.ipv4}</span>}
              {item.defaultRoute && <span style={{ fontSize: 10, color: '#888', marginLeft: 'auto' }}>↑ default</span>}
            </div>
          );
        }

        // Bridge group
        const bridgeColor = '#9B59B6';
        return (
          <React.Fragment key={`bridge-${item.name}`}>
            {/* Bridge header row — cosmetic only, no handle */}
            <div style={{
              display: 'flex', alignItems: 'center',
              padding: '5px 10px', background: '#1a1a1a',
              borderTop: '1px solid #2a2a2a',
              borderBottom: item.members.length ? '1px solid #222' : (isLastItem ? undefined : '1px solid #2a2a2a'),
              gap: 6,
            }}>
              <span style={{ fontSize: 10, color: bridgeColor }}>▣</span>
              <span style={{ fontSize: 12, color: bridgeColor, fontWeight: 700 }}>{item.name}</span>
              {item.ipv4 && <span style={{ fontSize: 10, color: '#777', marginLeft: 4 }}>{item.ipv4}</span>}
            </div>
            {/* Bridge member rows — left+right handles; indentation signals bridge membership */}
            {item.members.map((m, mi) => {
              const mColor = roleColor(m.role);
              const isLastMember = mi === item.members.length - 1;
              return (
                <div key={m.role} style={{
                  position: 'relative', display: 'flex', alignItems: 'center',
                  padding: '3px 28px 3px 36px',
                  borderBottom: (!isLastMember || !isLastItem) ? '1px solid #222' : undefined,
                  gap: 5,
                }}>
                  <Handle type="source" position={Position.Right} id={`iface-${m.role}`}
                    style={{ right: 4, width: 10, height: 10, background: mColor, border: `2px solid ${mColor}88` }} />
                  <Handle type="source" position={Position.Left} id={`iface-${m.role}-left`}
                    style={{ left: 4, width: 10, height: 10, background: mColor, border: `2px solid ${mColor}88` }} />
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: mColor, flexShrink: 0 }} />
                  <span style={{ fontSize: 11, color: mColor, fontWeight: 500 }}>{(m as { label: string }).label}</span>
                  {m.ipv4 && <span style={{ fontSize: 10, color: '#777', marginLeft: 2 }}>{m.ipv4}</span>}
                </div>
              );
            })}
          </React.Fragment>
        );
      })}
    </div>
  );
}
