// Shared TypeScript types used across extension host and webview.

export interface ServiceTypeDescriptor {
  name: string;
  description: string;
  defaultPullPolicy: string;
  defaultImage: string;
  expectedRoles: Array<{ role: string; required: boolean }>;
}

export interface DropTemplateInterface {
  role: string;
  device?: string;
  bridge?: string;
  sharing?: 'shared' | 'unique';
}

export interface DropTemplate {
  interfaces: DropTemplateInterface[];
  bridges?: Array<{ name: string; ipv4?: string }>;
}

export interface PaletteVariant {
  _variant: true;
  label: string;
  type: string;
  description?: string;
  interfaces: DropTemplateInterface[];
  bridges?: Array<{ name: string; ipv4?: string }>;
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
