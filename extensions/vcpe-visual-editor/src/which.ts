import { spawnSync } from 'child_process';
import * as path from 'path';

/**
 * Minimal cross-platform `which` implementation.
 * Returns the absolute path of the first matching executable on PATH, or null.
 */
export function which(name: string): string | null {
  const isWindows = process.platform === 'win32';
  const candidates = isWindows ? [name, name + '.exe', name + '.cmd'] : [name];

  const pathEnv = process.env.PATH ?? '';
  const dirs = pathEnv.split(path.delimiter);

  for (const dir of dirs) {
    for (const candidate of candidates) {
      const full = path.join(dir, candidate);
      const result = spawnSync(isWindows ? 'where' : 'which', [full], { encoding: 'utf8' });
      if (result.status === 0) {
        return full;
      }
    }
  }

  // Simpler fallback: just try the name directly and see if spawnSync succeeds
  const test = spawnSync(name, ['--help'], { encoding: 'utf8', timeout: 2_000 });
  if (!test.error) {
    return name;
  }

  return null;
}
