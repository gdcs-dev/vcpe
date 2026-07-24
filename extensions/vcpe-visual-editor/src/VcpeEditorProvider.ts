import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import { VcpeBinaryClient } from './VcpeBinaryClient';
import { ManifestScanner } from './ManifestScanner';
import { LayoutStore } from './LayoutStore';

/**
 * VcpeEditorProvider implements VS Code's CustomTextEditorProvider for
 * vcpe.dev/v1 manifests. It:
 *  - Opens a sandboxed React webview for the manifest file
 *  - Maintains an editInFlight mutex to prevent feedback loops during
 *    WorkspaceEdit application
 *  - Forwards YAML document changes → webview (DOCUMENT_UPDATED message)
 *  - Handles canvas mutation messages → WorkspaceEdit
 */
export class VcpeEditorProvider implements vscode.CustomTextEditorProvider {
  private readonly scanner: ManifestScanner;
  private readonly layoutStore: LayoutStore;

  constructor(
    private readonly context: vscode.ExtensionContext,
    private readonly log: vscode.OutputChannel,
    private readonly binaryClient: VcpeBinaryClient,
  ) {
    this.scanner = new ManifestScanner(log);
    this.layoutStore = new LayoutStore();
  }

  async resolveCustomTextEditor(
    document: vscode.TextDocument,
    webviewPanel: vscode.WebviewPanel,
  ): Promise<void> {
    const manifestPath = document.uri.fsPath;
    this.log.appendLine(`[VcpeEditorProvider] opening: ${manifestPath}`);

    webviewPanel.webview.options = {
      enableScripts: true,
      localResourceRoots: [
        vscode.Uri.joinPath(this.context.extensionUri, 'webview', 'dist'),
      ],
    };

    let editInFlight = false;

    // Initial render
    const types = this.binaryClient.getTypes();
    const layout = this.layoutStore.load(manifestPath);
    webviewPanel.webview.html = this.buildWebviewHtml(webviewPanel.webview);

    // Send initial data once the webview signals it is ready
    const sendInitialState = () => {
      webviewPanel.webview.postMessage({
        type: 'INIT',
        yaml: document.getText(),
        types: types instanceof Error ? [] : types,
        typesError: types instanceof Error ? types.message : null,
        layout,
        manifestPath,
      });
    };

    // Handle messages from the webview
    const messageDisposable = webviewPanel.webview.onDidReceiveMessage(async (msg) => {
      switch (msg.type) {
        case 'READY':
          sendInitialState();
          break;

        case 'CANVAS_MUTATION': {
          // Apply a WorkspaceEdit derived from the canvas change.
          // editInFlight prevents the resulting onDidChangeTextDocument from
          // triggering a re-render.
          editInFlight = true;
          try {
            const edit = new vscode.WorkspaceEdit();
            edit.replace(
              document.uri,
              new vscode.Range(0, 0, document.lineCount, 0),
              msg.newYaml,
            );
            await vscode.workspace.applyEdit(edit);
            this.log.appendLine(`[VcpeEditorProvider] applied WorkspaceEdit: ${msg.description ?? '(canvas mutation)'}`);
          } finally {
            editInFlight = false;
          }
          break;
        }

        case 'SAVE_LAYOUT': {
          this.layoutStore.save(manifestPath, msg.layout);
          this.log.appendLine(`[VcpeEditorProvider] saved layout for ${path.basename(manifestPath)}`);
          break;
        }

        case 'OPEN_MANIFEST': {
          const uri = vscode.Uri.file(msg.path);
          await vscode.commands.executeCommand('vscode.openWith', uri, 'vcpe.manifestEditor');
          break;
        }

        case 'CREATE_MANIFEST': {
          await this.createNewManifest(msg.name);
          break;
        }

        case 'REQUEST_MANIFEST_LIST': {
          const entries = await this.scanner.scan();
          webviewPanel.webview.postMessage({ type: 'MANIFEST_LIST', entries });
          break;
        }
      }
    });

    // Forward external YAML edits to the webview
    const changeDisposable = vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.uri.toString() !== document.uri.toString()) {
        return;
      }
      if (editInFlight) {
        return; // we caused this change — skip
      }
      this.log.appendLine(`[VcpeEditorProvider] external edit detected — re-parsing ${path.basename(manifestPath)}`);
      webviewPanel.webview.postMessage({
        type: 'DOCUMENT_UPDATED',
        yaml: document.getText(),
      });
    });

    webviewPanel.onDidDispose(() => {
      messageDisposable.dispose();
      changeDisposable.dispose();
    });
  }

  private async createNewManifest(name: string): Promise<void> {
    const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
    if (!workspaceFolder) {
      vscode.window.showErrorMessage('No workspace folder open.');
      return;
    }
    const manifestsDir = vscode.Uri.joinPath(workspaceFolder.uri, 'manifests');
    const newUri = vscode.Uri.joinPath(manifestsDir, `${name}.yaml`);

    const skeleton = [
      'apiVersion: vcpe.dev/v1',
      'kind: Deployment',
      'metadata:',
      `  name: ${name}`,
      'spec:',
      '  networks: []',
      '  services: []',
      '',
    ].join('\n');

    await vscode.workspace.fs.writeFile(newUri, Buffer.from(skeleton, 'utf8'));
    this.log.appendLine(`[VcpeEditorProvider] created new manifest: ${newUri.fsPath}`);
    await vscode.commands.executeCommand('vscode.openWith', newUri, 'vcpe.manifestEditor');
  }

  private buildWebviewHtml(webview: vscode.Webview): string {
    const distBase = vscode.Uri.joinPath(this.context.extensionUri, 'webview', 'dist');
    const htmlPath = path.join(distBase.fsPath, 'index.html');

    let html: string;
    try {
      html = fs.readFileSync(htmlPath, 'utf8');
    } catch {
      return `<html><body style="font-family:sans-serif;padding:20px;color:#f44">
        <strong>Error:</strong> webview bundle not found at <code>${htmlPath}</code>.<br>
        Run <code>make build-extension</code> then reload VS Code.
      </body></html>`;
    }

    const nonce = generateNonce();
    const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(distBase, 'index.js'));
    const styleUri  = webview.asWebviewUri(vscode.Uri.joinPath(distBase, 'index.css'));
    const csp = [
      `default-src 'none'`,
      `script-src 'nonce-${nonce}' ${webview.cspSource}`,
      `style-src ${webview.cspSource} 'unsafe-inline'`,
      `img-src ${webview.cspSource} data:`,
      `font-src ${webview.cspSource}`,
    ].join('; ');

    // Patch the Vite-built index.html:
    // 1. Replace absolute /index.js and /index.css with webview resource URIs
    // 2. Add nonce to the script tag (required by the CSP)
    // 3. Inject the Content-Security-Policy meta tag
    return html
      .replace(
        /(<script\b[^>]*)\ssrc="[^"]*index\.js"/,
        `$1 nonce="${nonce}" src="${scriptUri}"`
      )
      .replace(
        /(<link\b[^>]*)\shref="[^"]*index\.css"/,
        `$1 href="${styleUri}"`
      )
      .replace(
        '<head>',
        `<head>\n  <meta http-equiv="Content-Security-Policy" content="${csp}">`
      );
  }
}

function generateNonce(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
  let result = '';
  for (let i = 0; i < 32; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}
