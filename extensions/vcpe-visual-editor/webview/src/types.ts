// Shared TypeScript types used across extension host and webview.

export interface ServiceTypeDescriptor {
  name: string;
  description: string;
  defaultPullPolicy: string;
  defaultImage: string;
  expectedRoles: Array<{ role: string; required: boolean }>;
}

export interface NodePosition { x: number; y: number }

export interface LayoutData {
  version: 1;
  nodes: Record<string, NodePosition>;
  edgeHandles?: Record<string, { sourceHandle: string | null; targetHandle: string | null }>;
}

export interface ManifestEntry {
  name: string;
  path: string;
  description: string;
}
