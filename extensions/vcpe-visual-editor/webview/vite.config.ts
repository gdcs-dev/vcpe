import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  root: resolve(__dirname, 'src'),
  plugins: [react()],
  build: {
    outDir: resolve(__dirname, 'dist'),
    emptyOutDir: true,
    rollupOptions: {
      input: resolve(__dirname, 'src/index.html'),
      output: {
        // Predictable filenames — VcpeEditorProvider loads dist/index.js and dist/index.css.
        // No content hashes; the extension is rebuilt on every install anyway.
        entryFileNames: 'index.js',
        chunkFileNames: 'index.js',
        assetFileNames: (info) => {
          if (info.names?.some((n) => n.endsWith('.css'))) return 'index.css';
          return '[name][extname]';
        },
      },
    },
  },
  test: {
    globals: true,
    environment: 'node',
    include: ['**/*.test.ts'],
    root: resolve(__dirname, 'src'),
  },
});
