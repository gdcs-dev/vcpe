import { create } from 'zustand';
import type { ManifestModel } from './yaml/parse';
import type { ServiceTypeDescriptor, LayoutData, DropTemplate, PaletteVariant } from './types';

interface ManifestState {
  // Manifest state
  model: ManifestModel | null;
  yamlError: string | null;
  yamlErrorLine: number | undefined;

  // Type palette
  types: ServiceTypeDescriptor[];
  typesError: string | null;
  dropDefaults: Record<string, DropTemplate>;
  paletteVariants: PaletteVariant[];

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
  setDropDefaults: (defaults: Record<string, DropTemplate>) => void;
  setPaletteVariants: (variants: PaletteVariant[]) => void;
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
  dropDefaults: {},
  paletteVariants: [],
  layout: null,
  manifestPath: null,
  selectedNodeId: null,
  showDependsOn: false,

  setModel: (model) => set({ model, yamlError: null, yamlErrorLine: undefined }),
  setYamlError: (error, line) => set({ yamlError: error, yamlErrorLine: line, model: null }),
  setTypes: (types, typesError) => set({ types, typesError }),
  setDropDefaults: (dropDefaults) => set({ dropDefaults }),
  setPaletteVariants: (paletteVariants) => set({ paletteVariants }),
  setLayout: (layout) => set({ layout }),
  setManifestPath: (manifestPath) => set({ manifestPath }),
  selectNode: (selectedNodeId) => set({ selectedNodeId }),
  toggleDependsOn: () => set((s) => ({ showDependsOn: !s.showDependsOn })),
}));
