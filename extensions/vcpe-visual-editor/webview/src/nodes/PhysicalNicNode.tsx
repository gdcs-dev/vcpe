import React from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';

export interface PhysicalNicData {
  parent: string; // e.g. "eth0"
}

/**
 * PhysicalNicNode represents a physical host NIC that macvlan/ipvlan networks attach to.
 * Rendered as a distinct node above the macvlan NetworkBusNode(s).
 */
export function PhysicalNicNode({ data, selected }: NodeProps<PhysicalNicData>) {
  return (
    <div
      style={{
        padding: '6px 14px',
        borderRadius: 6,
        border: `1.5px solid ${selected ? '#4E9AF1' : '#666'}`,
        background: '#2a2a2a',
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        userSelect: 'none',
      }}
    >
      <span style={{ fontSize: 16 }}>🖥</span>
      <div>
        <div style={{ fontWeight: 600, fontSize: 12 }}>Physical NIC</div>
        <div style={{ fontSize: 11, color: '#888' }}>{data.parent}</div>
      </div>
      <Handle type="source" position={Position.Bottom} style={{ width: 8, height: 8 }} />
    </div>
  );
}
