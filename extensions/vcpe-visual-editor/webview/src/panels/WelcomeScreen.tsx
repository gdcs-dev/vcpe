import React from 'react';
import type { ManifestEntry } from '../types';

import { vscodeApi as vscode } from '../vsCodeApi';

interface Props {
  entries: ManifestEntry[];
}

/**
 * WelcomeScreen is shown when the editor opens without a specific manifest file.
 * It lists discovered workspace manifests and provides a "New Manifest" button.
 */
export function WelcomeScreen({ entries }: Props) {
  const handleOpen = (path: string) => {
    vscode?.postMessage({ type: 'OPEN_MANIFEST', path });
  };

  const handleNew = () => {
    const name = window.prompt('New manifest name (e.g. lab-test):')?.trim();
    if (name) vscode?.postMessage({ type: 'CREATE_MANIFEST', name });
  };

  return (
    <div style={styles.container}>
      <div style={styles.box}>
        <h2 style={styles.title}>vCPE Visual Manifest Editor</h2>
        <p style={styles.subtitle}>Open an existing manifest or create a new one.</p>

        {entries.length > 0 ? (
          <ul style={styles.list}>
            {entries.map(e => (
              <li key={e.path} style={styles.listItem} onClick={() => handleOpen(e.path)}>
                <span style={styles.listIcon}>📄</span>
                <div>
                  <div style={styles.listName}>{e.name}</div>
                  {e.description && <div style={styles.listDesc}>{e.description}</div>}
                </div>
              </li>
            ))}
          </ul>
        ) : (
          <p style={{ color: '#666', fontSize: 12 }}>No manifests found in workspace.</p>
        )}

        <button onClick={handleNew} style={styles.button}>＋ New Manifest</button>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  container: { flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' },
  box: { background: '#252526', border: '1px solid #444', borderRadius: 8, padding: 32, maxWidth: 480, width: '90%' },
  title: { fontSize: 18, fontWeight: 700, marginBottom: 8 },
  subtitle: { color: '#888', fontSize: 13, marginBottom: 20 },
  list: { listStyle: 'none', margin: '0 0 20px', padding: 0 },
  listItem: { display: 'flex', alignItems: 'flex-start', gap: 10, padding: '8px 10px', borderRadius: 4, cursor: 'pointer', marginBottom: 4, border: '1px solid transparent' },
  listIcon: { fontSize: 20, lineHeight: 1 },
  listName: { fontWeight: 600, fontSize: 13 },
  listDesc: { fontSize: 11, color: '#888' },
  button: { padding: '7px 16px', background: '#4E9AF1', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 13, fontWeight: 600 },
};
