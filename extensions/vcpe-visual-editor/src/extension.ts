import * as vscode from 'vscode';
import { VcpeEditorProvider } from './VcpeEditorProvider';
import { VcpeBinaryClient } from './VcpeBinaryClient';

let outputChannel: vscode.OutputChannel;
let binaryClient: VcpeBinaryClient;

export function activate(context: vscode.ExtensionContext): void {
  outputChannel = vscode.window.createOutputChannel('vCPE Visual Manifest Editor');
  outputChannel.appendLine('[extension] activating vCPE Visual Manifest Editor');

  binaryClient = new VcpeBinaryClient(outputChannel);

  const provider = new VcpeEditorProvider(context, outputChannel, binaryClient);
  context.subscriptions.push(
    vscode.window.registerCustomEditorProvider('vcpe.manifestEditor', provider, {
      webviewOptions: { retainContextWhenHidden: true },
      supportsMultipleEditorsPerDocument: false,
    })
  );

  outputChannel.appendLine('[extension] custom editor registered for vcpe.manifestEditor');
}

export function deactivate(): void {
  outputChannel?.dispose();
}
