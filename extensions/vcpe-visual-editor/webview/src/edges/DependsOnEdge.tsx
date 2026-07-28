import React from 'react';
import { EdgeProps, getBezierPath, EdgeLabelRenderer } from '@xyflow/react';
import { useManifestStore } from '../store/manifestStore';

/** Dashed gray arrow from dependent (A) to dependency (B): A → B = "A needs B". */
export function DependsOnEdge({
  id, sourceX, sourceY, targetX, targetY,
  sourcePosition, targetPosition, markerEnd,
}: EdgeProps) {
  const showDependsOn = useManifestStore((s) => s.showDependsOn);
  if (!showDependsOn) return null;

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition,
  });

  return (
    <>
      <path
        id={id}
        d={edgePath}
        fill="none"
        stroke="#666"
        strokeWidth={1.5}
        strokeDasharray="5 4"
        markerEnd={markerEnd}
        strokeOpacity={0.7}
      />
    </>
  );
}
