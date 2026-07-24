import { create } from 'zustand';
import type { ManifestModel } from './yaml/parse';
import type { ServiceTypeDescriptor } from './types';
import type { LayoutData } from './types';

interface ManifestState {
  // Manifest state
  model: ManifestModel | null;
  yamlError: string | null;
  yamlErrorLine: number | undefined;

  // Type palette
  types: ServiceTypeDescriptor[];
  typesError: string | null;

  // Layout
  layout: LayoutData | null;
  manifestPath: string | null;

  // Canvas UI state
  selectedNodeId: string | null;
  showDependsOn: boolean;

  // Actions
  setModel: (model: ManifestModel, yaml: string) => void;
  setYamlError: (error: string, line?: number) => void;
  setTypes: (types: ServiceTypeDescriptor[], error: string | null) => void;
  setLayout: (layout: LayoutData) => void;
  setManifestPath: (path: string) => void;
  selectNode: (id: string | null) => void;
  toggleDependsOn: () => void;
}

export const useManifestStore = create<ManifestState>((set) => ({
  model: null,
  yamlError: null,
  yamlErrorLine: undefined,
  types: [],
  typesError: null,
  layout: null,
  manifestPath: null,
  selectedNodeId: null,
  showDependsOn: true,

  setModel: (model) => set({ model, yamlError: null, yamlErrorLine: undefined }),
  setYamlError: (error, line) => set({ yamlError: error, yamlErrorLine: line, model: null }),
  setTypes: (types, typesError) => set({ types, typesError }),
  setLayout: (layout) => set({ layout }),
  setManifestPath: (manifestPath) => set({ manifestPath }),
  selectNode: (selectedNodeId) => set({ selectedNodeId }),
  toggleDependsOn: () => set((s) => ({ showDependsOn: !s.showDependsOn })),
}));
