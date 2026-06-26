package persist

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// SchemaVersion stamps the on-disk state so a control plane refuses to operate
// against state written by an incompatible schema.
const SchemaVersion = "vcpe.dev/v1"

type IPAMLease struct {
	CustomerID string
	Role       string
	CIDR       string
}

type OperationTimelineEntry struct {
	OperationID string `json:"operationId"`
	Command     string `json:"command"`
	Status      string `json:"status"`
	UpdatedAt   string `json:"updatedAt"`
}

type MetricsSnapshot struct {
	ReconcileTotal      int `json:"reconcileTotal"`
	ReconcileFailures   int `json:"reconcileFailures"`
	IPAMLeasesInUse     int `json:"ipamLeasesInUse"`
	DriftCount          int `json:"driftCount"`
	RunningOperations   int `json:"runningOperations"`
	RecoveredOperations int `json:"recoveredOperations"`
}

type OperationPhaseEntry struct {
	Phase   string `json:"phase"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func Open(stateRoot string) (*Store, error) {
	dbPath := filepath.Join(stateRoot, "state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite state db: %w", err)
	}

	s := &Store{db: db}
	if err := s.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.ensureSchemaVersion(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ensureSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS operations (
  operation_id TEXT PRIMARY KEY,
  command TEXT NOT NULL,
  manifest_path TEXT,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS operation_journal (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  operation_id TEXT NOT NULL,
  phase TEXT NOT NULL,
  status TEXT NOT NULL,
  message TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS desired_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  customer_id TEXT NOT NULL,
  manifest TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ipam_leases (
  customer_id TEXT NOT NULL,
  role TEXT NOT NULL,
  cidr TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(customer_id, role)
);

CREATE TABLE IF NOT EXISTS checkpoints (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("initialize sqlite schema: %w", err)
	}
	return nil
}

// ensureSchemaVersion stamps a fresh database and refuses to open state written
// by a different schema version.
func (s *Store) ensureSchemaVersion() error {
	var version string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version)
	if err == sql.ErrNoRows {
		if _, err := s.db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?)`, SchemaVersion); err != nil {
			return fmt.Errorf("stamp schema version: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version != SchemaVersion {
		return fmt.Errorf("state schema version %q is incompatible with %q: run `vcpe state reset` to reinitialize", version, SchemaVersion)
	}
	return nil
}

// Reset clears all persisted state and re-stamps the schema version. It backs
// the `vcpe state reset` command.
func (s *Store) Reset() error {
	tables := []string{"operations", "operation_journal", "desired_snapshots", "ipam_leases", "checkpoints", "meta"}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reset tx: %w", err)
	}
	for _, table := range tables {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("reset table %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?)`, SchemaVersion); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("re-stamp schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reset tx: %w", err)
	}
	return nil
}

func (s *Store) StartOperation(command, manifestPath string) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	opID := fmt.Sprintf("op-%d", time.Now().UnixNano())
	if _, err := s.db.Exec(
		`INSERT INTO operations(operation_id, command, manifest_path, status, created_at, updated_at) VALUES(?, ?, ?, 'running', ?, ?)`,
		opID, command, manifestPath, now, now,
	); err != nil {
		return "", fmt.Errorf("start operation: %w", err)
	}
	if err := s.RecordPhase(opID, "operation", "started", "operation started"); err != nil {
		return "", err
	}
	return opID, nil
}

func (s *Store) RecordPhase(opID, phase, status, message string) error {
	_, err := s.db.Exec(
		`INSERT INTO operation_journal(operation_id, phase, status, message, created_at) VALUES(?, ?, ?, ?, ?)`,
		opID, phase, status, message, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("record phase %s: %w", phase, err)
	}
	return nil
}

func (s *Store) FinishOperation(opID, status, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE operations SET status = ?, updated_at = ? WHERE operation_id = ?`, status, now, opID); err != nil {
		return fmt.Errorf("finish operation: %w", err)
	}
	if err := s.RecordPhase(opID, "operation", status, message); err != nil {
		return err
	}
	return nil
}

func (s *Store) SaveDesiredSnapshot(customerID string, manifest []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO desired_snapshots(customer_id, manifest, created_at) VALUES(?, ?, ?)`,
		customerID, string(manifest), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save desired snapshot: %w", err)
	}
	return nil
}

func (s *Store) ListIPAMLeases() ([]IPAMLease, error) {
	rows, err := s.db.Query(`SELECT customer_id, role, cidr FROM ipam_leases`)
	if err != nil {
		return nil, fmt.Errorf("query ipam leases: %w", err)
	}
	defer rows.Close()

	out := []IPAMLease{}
	for rows.Next() {
		var l IPAMLease
		if err := rows.Scan(&l.CustomerID, &l.Role, &l.CIDR); err != nil {
			return nil, fmt.Errorf("scan ipam lease: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ipam leases: %w", err)
	}
	return out, nil
}

func (s *Store) ReplaceCustomerLeases(customerID string, leases []IPAMLease) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin lease replace tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM ipam_leases WHERE customer_id = ?`, customerID); err != nil {
		return fmt.Errorf("delete existing customer leases: %w", err)
	}

	for _, l := range leases {
		if _, err = tx.Exec(
			`INSERT INTO ipam_leases(customer_id, role, cidr, updated_at) VALUES(?, ?, ?, ?)`,
			l.CustomerID, l.Role, l.CIDR, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("insert customer lease %s/%s: %w", l.CustomerID, l.Role, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit lease replace tx: %w", err)
	}
	return nil
}

func (s *Store) UpsertCheckpoint(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO checkpoints(key, value, updated_at) VALUES(?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert checkpoint: %w", err)
	}
	return nil
}

func (s *Store) RecoverUnfinishedOperations() (int, error) {
	rows, err := s.db.Query(`SELECT operation_id FROM operations WHERE status = 'running'`)
	if err != nil {
		return 0, fmt.Errorf("query running operations: %w", err)
	}
	defer rows.Close()

	recovered := []string{}
	for rows.Next() {
		var opID string
		if err := rows.Scan(&opID); err != nil {
			return 0, fmt.Errorf("scan running operation: %w", err)
		}
		recovered = append(recovered, opID)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate running operations: %w", err)
	}

	for _, opID := range recovered {
		if _, err := s.db.Exec(
			`UPDATE operations SET status = 'failed_recovered', updated_at = ? WHERE operation_id = ?`,
			time.Now().UTC().Format(time.RFC3339Nano), opID,
		); err != nil {
			return 0, fmt.Errorf("mark recovered operation %s: %w", opID, err)
		}
		if err := s.RecordPhase(opID, "recovery", "failed_recovered", "operation was running at startup and was marked failed"); err != nil {
			return 0, err
		}
	}

	if len(recovered) > 0 {
		if err := s.UpsertCheckpoint("last_recovery", fmt.Sprintf("%d", len(recovered))); err != nil {
			return 0, err
		}
	}
	return len(recovered), nil
}

func (s *Store) RecentOperations(limit int) ([]OperationTimelineEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT operation_id, command, status, updated_at FROM operations ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent operations: %w", err)
	}
	defer rows.Close()

	out := []OperationTimelineEntry{}
	for rows.Next() {
		var e OperationTimelineEntry
		if err := rows.Scan(&e.OperationID, &e.Command, &e.Status, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan operation timeline: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation timeline: %w", err)
	}
	return out, nil
}

func (s *Store) Metrics() (MetricsSnapshot, error) {
	metrics := MetricsSnapshot{}

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM operations WHERE command = 'apply'`).Scan(&metrics.ReconcileTotal); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query reconcile total: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM operations WHERE command = 'apply' AND status != 'succeeded'`).Scan(&metrics.ReconcileFailures); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query reconcile failures: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ipam_leases`).Scan(&metrics.IPAMLeasesInUse); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query ipam leases in use: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM operations WHERE status = 'running'`).Scan(&metrics.RunningOperations); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query running operations: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(CAST(value AS INTEGER)), 0) FROM checkpoints WHERE key = 'last_recovery'`).Scan(&metrics.RecoveredOperations); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query recovered operations: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM operations WHERE status LIKE '%drift%'`).Scan(&metrics.DriftCount); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query drift count: %w", err)
	}

	return metrics, nil
}

// ListKnownDeployments returns the deployment names (metadata.name) that
// currently have active IPAM leases. A deployment disappears from this list
// once vcpe down releases its leases.
func (s *Store) ListKnownDeployments() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT customer_id FROM ipam_leases ORDER BY customer_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list known deployments: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan deployment name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// DeleteDeploymentSnapshot removes all desired-state snapshots for the named
// deployment. Called by vcpe down so torn-down deployments do not reappear in
// history queries.
func (s *Store) DeleteDeploymentSnapshot(customerID string) error {
	if _, err := s.db.Exec(`DELETE FROM desired_snapshots WHERE customer_id = ?`, customerID); err != nil {
		return fmt.Errorf("delete deployment snapshot %s: %w", customerID, err)
	}
	return nil
}

func (s *Store) CountKnownCustomers() (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT DISTINCT customer_id FROM desired_snapshots
			UNION
			SELECT DISTINCT customer_id FROM ipam_leases
		)
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count known customers: %w", err)
	}
	return count, nil
}

func (s *Store) CustomerExists(customerID string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM desired_snapshots WHERE customer_id = ?
			UNION
			SELECT 1 FROM ipam_leases WHERE customer_id = ?
		)
	`, customerID, customerID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check known customer: %w", err)
	}
	return exists == 1, nil
}

func (s *Store) OperationPhases(operationID string) ([]OperationPhaseEntry, error) {
	rows, err := s.db.Query(`SELECT phase, status, COALESCE(message, '') FROM operation_journal WHERE operation_id = ? ORDER BY id ASC`, operationID)
	if err != nil {
		return nil, fmt.Errorf("query operation phases: %w", err)
	}
	defer rows.Close()
	out := []OperationPhaseEntry{}
	for rows.Next() {
		var entry OperationPhaseEntry
		if err := rows.Scan(&entry.Phase, &entry.Status, &entry.Message); err != nil {
			return nil, fmt.Errorf("scan operation phase: %w", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation phases: %w", err)
	}
	return out, nil
}

func (s *Store) LatestDesiredSnapshot(customerID string) ([]byte, bool, error) {
	if customerID == "" {
		return nil, false, nil
	}
	var manifestText string
	err := s.db.QueryRow(
		`SELECT manifest FROM desired_snapshots WHERE customer_id = ? ORDER BY id DESC LIMIT 1`,
		customerID,
	).Scan(&manifestText)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("query latest desired snapshot: %w", err)
	}
	return []byte(manifestText), true, nil
}
