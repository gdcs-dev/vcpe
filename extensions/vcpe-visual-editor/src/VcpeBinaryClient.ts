import * as vscode from 'vscode';
import { spawnSync } from 'child_process';
import { which } from './which';

export interface RoleRequirement {
  role: string;
  required: boolean;
}

export interface ServiceTypeDescriptor {
  name: string;
  description: string;
  defaultPullPolicy: string;
  defaultImage: string;
  expectedRoles: RoleRequirement[];
}

interface ServiceTypesResponse {
  types: ServiceTypeDescriptor[];
}

/**
 * VcpeBinaryClient locates the vcpe binary and fetches registered service types.
 * Types are cached in memory for the lifetime of the extension session.
 */
export class VcpeBinaryClient {
  private cache: ServiceTypeDescriptor[] | null = null;

  constructor(private readonly log: vscode.OutputChannel) {}

  /**
   * Returns the list of registered service types from `vcpe service types --json`.
   * Uses the vcpe.binaryPath setting if set, otherwise falls back to PATH lookup.
   * Returns an empty array and logs an error if the binary cannot be found.
   */
  getTypes(): ServiceTypeDescriptor[] | Error {
    if (this.cache !== null) {
      return this.cache;
    }

    const binary = this.resolveBinary();
    if (binary instanceof Error) {
      this.log.appendLine(`[VcpeBinaryClient] ${binary.message}`);
      return binary;
    }

    this.log.appendLine(`[VcpeBinaryClient] running: ${binary} service types --json`);
    const result = spawnSync(binary, ['service', 'types', '--json'], {
      encoding: 'utf8',
      timeout: 10_000,
    });

    if (result.error) {
      const msg = `failed to run vcpe binary at "${binary}": ${result.error.message}`;
      this.log.appendLine(`[VcpeBinaryClient] ${msg}`);
      return new Error(msg);
    }
    if (result.status !== 0) {
      const msg = `vcpe service types --json exited ${result.status}: ${result.stderr}`;
      this.log.appendLine(`[VcpeBinaryClient] ${msg}`);
      return new Error(msg);
    }

    try {
      const parsed: ServiceTypesResponse = JSON.parse(result.stdout);
      this.cache = parsed.types ?? [];
      this.log.appendLine(`[VcpeBinaryClient] cached ${this.cache.length} service types`);
      return this.cache;
    } catch (e) {
      const msg = `failed to parse vcpe service types output: ${e}`;
      this.log.appendLine(`[VcpeBinaryClient] ${msg}`);
      return new Error(msg);
    }
  }

  /** Resolve the vcpe binary path: configured setting → PATH lookup. */
  private resolveBinary(): string | Error {
    const configured = vscode.workspace
      .getConfiguration('vcpe')
      .get<string>('binaryPath', '')
      .trim();

    if (configured) {
      this.log.appendLine(`[VcpeBinaryClient] using configured binaryPath: ${configured}`);
      return configured;
    }

    const found = which('vcpe');
    if (found) {
      this.log.appendLine(`[VcpeBinaryClient] found vcpe on PATH: ${found}`);
      return found;
    }

    return new Error(
      'vcpe binary not found. Set "vcpe.binaryPath" in VS Code settings to the full path of the vcpe binary.'
    );
  }
}
