import * as fs from 'fs';
import * as path from 'path';

export interface NodePosition {
  x: number;
  y: number;
}

export interface LayoutData {
  version: 1;
  nodes: Record<string, NodePosition>;
}

/**
 * LayoutStore reads and writes .vcpe-layout.json sidecar files.
 * The sidecar lives alongside the manifest: manifests/example.vcpe-layout.json
 * Schema: { version: 1, nodes: { "<kind>:<id>": { x, y } } }
 */
export class LayoutStore {
  private sidecarPath(manifestPath: string): string {
    const dir = path.dirname(manifestPath);
    const base = path.basename(manifestPath, '.yaml');
    return path.join(dir, `${base}.vcpe-layout.json`);
  }

  load(manifestPath: string): LayoutData | null {
    const sidecar = this.sidecarPath(manifestPath);
    if (!fs.existsSync(sidecar)) {
      return null;
    }
    try {
      const raw = fs.readFileSync(sidecar, 'utf8');
      const data = JSON.parse(raw) as LayoutData;
      if (data.version !== 1 || typeof data.nodes !== 'object') {
        return null;
      }
      return data;
    } catch {
      return null;
    }
  }

  save(manifestPath: string, layout: LayoutData): void {
    const sidecar = this.sidecarPath(manifestPath);
    fs.writeFileSync(sidecar, JSON.stringify(layout, null, 2) + '\n', 'utf8');
  }
}
