import React from 'react';
import type { ServiceTypeDescriptor } from '../types';

interface Props {
  types: ServiceTypeDescriptor[];
  typesError: string | null;
  onDrop?: (type: ServiceTypeDescriptor, position: { x: number; y: number }) => void;
}

/**
 * TypePalette renders draggable service type cards populated from vcpe service types --json.
 */
export function TypePalette({ types, typesError }: Props) {
  if (typesError) {
    return (
      <div style={styles.container}>
        <div style={styles.header}>Service Types</div>
        <div style={styles.error}>
          <strong>⚠ vcpe binary not found</strong>
          <p style={{ marginTop: 6, fontSize: 11 }}>{typesError}</p>
          <p style={{ marginTop: 4, fontSize: 11, color: '#888' }}>
            Set <code>vcpe.binaryPath</code> in VS Code settings.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div style={styles.container}>
      <div style={styles.header}>Service Types</div>
      {types.map((t) => (
        <div
          key={t.name}
          draggable
          onDragStart={(e) => {
            e.dataTransfer.setData('application/vcpe-service-type', JSON.stringify(t));
            e.dataTransfer.effectAllowed = 'copy';
          }}
          style={styles.card}
          title={t.description}
        >
          <div style={styles.cardName}>{t.name}</div>
          <div style={styles.cardDesc}>{t.description}</div>
          {t.expectedRoles.length > 0 && (
            <div style={styles.cardRoles}>
              {t.expectedRoles.map(r => (
                <span key={r.role} style={{ ...styles.roleTag, opacity: r.required ? 1 : 0.6 }}>
                  {r.role}{r.required ? '' : '?'}
                </span>
              ))}
            </div>
          )}
        </div>
      ))}
      {types.length === 0 && (
        <div style={{ padding: 12, color: '#666', fontSize: 11 }}>Loading…</div>
      )}
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  container: { width: 180, padding: 8, borderRight: '1px solid #333', overflowY: 'auto', background: 'var(--vscode-sideBar-background, #252526)' },
  header: { fontWeight: 700, fontSize: 11, color: '#888', textTransform: 'uppercase', padding: '4px 4px 8px', letterSpacing: 0.5 },
  card: { padding: '6px 8px', marginBottom: 6, borderRadius: 4, border: '1px solid #444', cursor: 'grab', background: '#2d2d2d', userSelect: 'none' },
  cardName: { fontWeight: 600, fontSize: 12, marginBottom: 2 },
  cardDesc: { fontSize: 10, color: '#888', lineHeight: 1.4 },
  cardRoles: { marginTop: 4, display: 'flex', flexWrap: 'wrap', gap: 3 },
  roleTag: { fontSize: 9, padding: '1px 4px', borderRadius: 2, background: '#4E9AF122', color: '#4E9AF1', border: '1px solid #4E9AF133' },
  error: { padding: 10, fontSize: 12, color: '#E74C3C', borderRadius: 4, border: '1px solid #E74C3C44', background: '#E74C3C11' },
};
