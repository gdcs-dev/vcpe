import React, { useEffect, useState } from 'react';
import type { ManifestEntry } from '../types';

import { vscodeApi as vscode } from '../vsCodeApi';

interface Props {
  currentPath: string | null;
}

/**
 * ManifestDropdown renders a toolbar dropdown that lists workspace manifests
 * and provides a "New Manifest…" action.
 */
export function ManifestDropdown({ currentPath }: Props) {
  const [entries, setEntries] = useState<ManifestEntry[]>([]);

  useEffect(() => {
    // Request manifest list from extension host
    vscode?.postMessage({ type: 'REQUEST_MANIFEST_LIST' });

    const handler = (event: MessageEvent) => {
      const msg = event.data;
      if (msg?.type === 'MANIFEST_LIST') {
        setEntries(msg.entries ?? []);
      }
    };
    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, []);

  const handleChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const value = e.target.value;
    if (value === '__new__') {
      const name = window.prompt('New manifest name (e.g. lab-test):')?.trim();
      if (name) {
        vscode?.postMessage({ type: 'CREATE_MANIFEST', name });
      }
    } else if (value && value !== currentPath) {
      vscode?.postMessage({ type: 'OPEN_MANIFEST', path: value });
    }
  };

  return (
    <select
      value={currentPath ?? ''}
      onChange={handleChange}
      style={{
        padding: '2px 6px', fontSize: 12, borderRadius: 4,
        background: '#2d2d2d', color: '#ccc', border: '1px solid #555',
        maxWidth: 220,
      }}
    >
      {currentPath && !entries.find(e => e.path === currentPath) && (
        <option value={currentPath}>{currentPath.split('/').pop()}</option>
      )}
      {entries.map(e => (
        <option key={e.path} value={e.path}>{e.name}</option>
      ))}
      <option value="__new__">＋ New Manifest…</option>
    </select>
  );
}
