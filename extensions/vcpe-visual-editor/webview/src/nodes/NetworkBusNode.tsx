import React from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { roleColor } from '../utils/roleColor';

export interface NetworkBusData {
  role: string;
  cidr4?: string;
  cidr6?: string;
  nat?: boolean;
  firewall?: boolean;
  driver?: string;
  isMacvlan?: boolean;
}

/**
 * NetworkBusNode renders as a wide horizontal lane representing a network segment.
 * Services connect to it via InterfaceEdge lines.
 */
export function NetworkBusNode({ data, selected }: NodeProps<NetworkBusData>) {
  const color = roleColor(data.role);
  return (
    <div
      style={{
        width: '100%',
        minWidth: 600,
        height: 48,
        borderRadius: 6,
        border: `2px solid ${color}`,
        borderLeft: `6px solid ${color}`,
        background: selected ? `${color}22` : `${color}11`,
        display: 'flex',
        alignItems: 'center',
        padding: '0 12px',
        gap: 10,
        userSelect: 'none',
      }}
    >
      <Handle type="target" position={Position.Top} style={{ opacity: 0 }} />
      <span style={{ fontWeight: 700, color, fontSize: 13, minWidth: 80 }}>{data.role}</span>
      {data.cidr4 && <span style={{ color: '#888', fontSize: 11 }}>{data.cidr4}</span>}
      {data.cidr6 && <span style={{ color: '#888', fontSize: 11 }}>{data.cidr6}</span>}
      {data.nat && <Badge label="nat" color="#E67E22" />}
      {data.firewall && <Badge label="fw" color="#E74C3C" />}
      {data.driver && data.driver !== 'bridge' && (
        <Badge label={data.driver} color="#9B59B6" />
      )}
      <Handle type="source" position={Position.Bottom} style={{ opacity: 0 }} />
    </div>
  );
}

function Badge({ label, color }: { label: string; color: string }) {
  return (
    <span
      style={{
        fontSize: 10, padding: '1px 5px', borderRadius: 3,
        background: `${color}22`, color, border: `1px solid ${color}55`,
        fontWeight: 600,
      }}
    >
      {label}
    </span>
  );
}
