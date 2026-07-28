import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';

export interface ManifestEntry {
  name: string;
  path: string;
  description: string;
}

const VCPE_API_VERSION = 'vcpe.dev/v1';

/**
 * ManifestScanner discovers vcpe.dev/v1 manifests within VS Code workspace folders.
 * It globs for manifests matching "** /manifests/*.yaml" and sniffs the first few lines for apiVersion.
 */
export class ManifestScanner {
  constructor(private readonly log: vscode.OutputChannel) {}

  async scan(): Promise<ManifestEntry[]> {
    const uris = await vscode.workspace.findFiles('**/manifests/*.yaml', '**/node_modules/**');
    const entries: ManifestEntry[] = [];

    for (const uri of uris) {
      try {
        const entry = this.sniff(uri.fsPath);
        if (entry) {
          entries.push(entry);
        }
      } catch (e) {
        this.log.appendLine(`[ManifestScanner] skipping ${uri.fsPath}: ${e}`);
      }
    }

    this.log.appendLine(`[ManifestScanner] found ${entries.length} vcpe.dev/v1 manifest(s)`);
    return entries;
  }

  private sniff(filePath: string): ManifestEntry | null {
    // Read first 512 bytes — enough to find apiVersion without loading large files.
    const fd = fs.openSync(filePath, 'r');
    const buf = Buffer.alloc(512);
    const bytesRead = fs.readSync(fd, buf, 0, 512, 0);
    fs.closeSync(fd);

    const head = buf.slice(0, bytesRead).toString('utf8');
    if (!head.includes(`apiVersion: ${VCPE_API_VERSION}`)) {
      return null;
    }

    // Extract name from metadata.name line (best-effort; no full YAML parse).
    const nameMatch = head.match(/^\s*name:\s*(.+)$/m);
    const name = nameMatch ? nameMatch[1].trim() : path.basename(filePath, '.yaml');

    // Extract description from annotations.description line (best-effort).
    const descMatch = head.match(/description:\s*["']?([^"'\n]+)["']?/);
    const description = descMatch ? descMatch[1].trim() : '';

    return { name, path: filePath, description };
  }
}
