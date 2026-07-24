// esbuild.js — bundles the VS Code extension host entry point.
// The webview React app is bundled separately by Vite (see webview/vite.config.ts).
const esbuild = require('esbuild');

const isWatch = process.argv.includes('--watch');

/** @type {import('esbuild').BuildOptions} */
const options = {
  entryPoints: ['src/extension.ts'],
  bundle: true,
  outfile: 'dist/extension.js',
  external: ['vscode'],   // VS Code API is provided at runtime, not bundled
  format: 'cjs',
  platform: 'node',
  target: 'node18',
  sourcemap: true,
  minify: false,
};

if (isWatch) {
  esbuild.context(options).then(ctx => ctx.watch());
} else {
  esbuild.build(options).catch(() => process.exit(1));
}
