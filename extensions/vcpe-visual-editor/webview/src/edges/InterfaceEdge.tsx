import React from 'react';
import { EdgeProps, getBezierPath } from '@xyflow/react';
import { roleColor } from '../utils/roleColor';

export interface InterfaceEdgeData {
  role: string;
  cidr?: string;
}

/** Solid colored bezier edge between two ServiceNodes sharing a network role. */
export function InterfaceEdge({
  id, sourceX, sourceY, targetX, targetY,
  sourcePosition, targetPosition, data,
}: EdgeProps<InterfaceEdgeData>) {
  const [edgePath] = getBezierPath({
    sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition,
  });
  const color = roleColor(data?.role ?? '');

  return (
    <path
      id={id}
      d={edgePath}
      fill="none"
      stroke={color}
      strokeWidth={2}
      strokeOpacity={0.8}
    />
  );
}
