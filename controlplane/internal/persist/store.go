package persist

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// SchemaVersion stamps the on-disk state so a control plane refuses to operate
// against state written by an incompatible schema.
const SchemaVersion = "vcpe.dev/state/v3"

type IPAMLease struct {
	CustomerID string
	Role       string
	CIDR       string
}

const (
	HealthPortMin = 47000
	HealthPortMax = 47999
)

// HealthEndpoint is a loopback-published health endpoint reserved for one
// deployment service replica.
type HealthEndpoint struct {
	Deployment string
	Service    string
	Replica    int
	HostPort   int
}

// WirelessGroup is one deployment's stable hwsim group-bit allocation.
type WirelessGroup struct {
	Deployment string `json:"deployment"`
	Bit        int    `json:"bit"`
}

// Mask returns the nonzero hwsim group mask for this allocation.
func (g WirelessGroup) Mask() uint64 { return uint64(1) << uint(g.Bit) }

// WirelessRadio is persisted desired ownership for one service replica radio.
type WirelessRadio struct {
	Deployment    string `json:"deployment"`
	Service       string `json:"service"`
	Replica       int    `json:"replica"`
	LogicalName   string `json:"logicalName"`
	ManagerName   string `json:"managerName"`
	MAC           string `json:"mac"`
	Network       string `json:"network"`
	Device        string `json:"device"`
	Mode          string `json:"mode"`
	Bridge        string `json:"bridge,omitempty"`
	ContainerName string `json:"containerName"`
	GroupBit      int    `json:"groupBit"`
	Status        string `json:"status"`
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
	WirelessGroups      int `json:"wirelessGroups"`
	WirelessRadios      int `json:"wirelessRadios"`
}

type OperationPhaseEntry struct {
	Phase   string `json:"phase"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func Open(stateRoot string) (*Store, error) {
	return open(stateRoot, true)
}

// OpenForReset opens the database without enforcing its current schema stamp.
// It is restricted to the explicit state-reset path, which immediately clears
// and re-stamps all tables.
func OpenForReset(stateRoot string) (*Store, error) {
	return open(stateRoot, false)
}

func open(stateRoot string, enforceVersion bool) (*Store, error) {
	dbPath := filepath.Join(stateRoot, "state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite state db: %w", err)
	}

	s := &Store{db: db}
	db.SetMaxOpenConns(1)
	if err := s.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if enforceVersion {
		if err := s.ensureSchemaVersion(); err != nil {
			_ = db.Close()
			return nil, err
		}
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

CREATE TABLE IF NOT EXISTS health_endpoints (
	customer_id TEXT NOT NULL,
	service_name TEXT NOT NULL,
	replica_index INTEGER NOT NULL,
	host_port INTEGER NOT NULL UNIQUE,
	PRIMARY KEY(customer_id, service_name, replica_index)
);

CREATE TABLE IF NOT EXISTS wireless_groups (
	customer_id TEXT PRIMARY KEY,
	bit_index INTEGER NOT NULL UNIQUE CHECK(bit_index >= 0 AND bit_index < 64),
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wireless_radios (
	customer_id TEXT NOT NULL,
	service_name TEXT NOT NULL,
	replica_index INTEGER NOT NULL,
	logical_name TEXT NOT NULL,
	manager_name TEXT NOT NULL UNIQUE,
	mac TEXT NOT NULL,
	network_name TEXT NOT NULL,
	device_name TEXT NOT NULL,
	mode TEXT NOT NULL,
	bridge_name TEXT NOT NULL,
	container_name TEXT NOT NULL,
	group_bit INTEGER NOT NULL,
	status TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY(customer_id, service_name, replica_index, logical_name),
	FOREIGN KEY(customer_id) REFERENCES wireless_groups(customer_id)
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
	tables := []string{"operations", "operation_journal", "desired_snapshots", "ipam_leases", "health_endpoints", "wireless_radios", "wireless_groups", "checkpoints", "meta"}
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
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM wireless_groups`).Scan(&metrics.WirelessGroups); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query wireless groups: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM wireless_radios`).Scan(&metrics.WirelessRadios); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("query wireless radios: %w", err)
	}

	return metrics, nil
}

// ListKnownDeployments returns the deployment names (metadata.name) that have
// an active desired snapshot or IPAM leases. A deployment disappears from this
// list once vcpe down clears both records.
func (s *Store) ListKnownDeployments() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT customer_id FROM ipam_leases
		UNION
		SELECT DISTINCT customer_id FROM desired_snapshots
		UNION
		SELECT DISTINCT customer_id FROM wireless_groups
		ORDER BY customer_id ASC
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

// GetReplicaCount returns the replica count from the last successful apply for
// the given deployment/service pair. Returns 0 when no prior apply exists
// (the migration/fallback: treat as "no prior apply" causing a full deploy).
func (s *Store) GetReplicaCount(deployment, service string) (int, error) {
	key := "replica_count/" + deployment + "/" + service
	var value string
	err := s.db.QueryRow(`SELECT value FROM checkpoints WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get replica count %s/%s: %w", deployment, service, err)
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0, nil // malformed value — treat as no prior apply
	}
	return count, nil
}

// SetReplicaCount persists the applied replica count for a deployment/service
// pair. Called after each successful apply to track the live baseline.
func (s *Store) SetReplicaCount(deployment, service string, count int) error {
	return s.UpsertCheckpoint("replica_count/"+deployment+"/"+service, strconv.Itoa(count))
}

// DeleteReplicaCounts removes all persisted replica counts for a deployment.
// Called by vcpe down so future applies start fresh.
func (s *Store) DeleteReplicaCounts(deployment string) error {
	prefix := "replica_count/" + deployment + "/%"
	if _, err := s.db.Exec(`DELETE FROM checkpoints WHERE key LIKE ?`, prefix); err != nil {
		return fmt.Errorf("delete replica counts for %s: %w", deployment, err)
	}
	return nil
}

func (s *Store) SetPendingCredentialRecreate(deployment string, services []string) error {
	encoded, err := json.Marshal(services)
	if err != nil {
		return fmt.Errorf("encode pending credential recreation: %w", err)
	}
	return s.UpsertCheckpoint("credential_recreate/"+deployment, string(encoded))
}

func (s *Store) PendingCredentialRecreate(deployment string) ([]string, error) {
	var encoded string
	err := s.db.QueryRow(`SELECT value FROM checkpoints WHERE key = ?`, "credential_recreate/"+deployment).Scan(&encoded)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get pending credential recreation for %s: %w", deployment, err)
	}
	var services []string
	if err := json.Unmarshal([]byte(encoded), &services); err != nil {
		return nil, fmt.Errorf("decode pending credential recreation for %s: %w", deployment, err)
	}
	return services, nil
}

func (s *Store) DeletePendingCredentialRecreate(deployment string) error {
	if _, err := s.db.Exec(`DELETE FROM checkpoints WHERE key = ?`, "credential_recreate/"+deployment); err != nil {
		return fmt.Errorf("delete pending credential recreation for %s: %w", deployment, err)
	}
	return nil
}

// ReserveHealthEndpoint returns a stable loopback host-port reservation for a
// deployment service replica. Reservations remain stable across reconciles and
// are globally unique inside the control-plane-owned range.
func (s *Store) ReserveHealthEndpoint(deployment, service string, replica int) (HealthEndpoint, error) {
	if deployment == "" || service == "" || replica < 0 {
		return HealthEndpoint{}, fmt.Errorf("invalid health endpoint identity %q/%q/%d", deployment, service, replica)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return HealthEndpoint{}, fmt.Errorf("begin health endpoint reservation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	endpoint := HealthEndpoint{Deployment: deployment, Service: service, Replica: replica}
	err = tx.QueryRow(`
		SELECT host_port FROM health_endpoints
		WHERE customer_id = ? AND service_name = ? AND replica_index = ?
	`, deployment, service, replica).Scan(&endpoint.HostPort)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return HealthEndpoint{}, fmt.Errorf("commit health endpoint reservation: %w", err)
		}
		return endpoint, nil
	}
	if err != sql.ErrNoRows {
		return HealthEndpoint{}, fmt.Errorf("lookup health endpoint %s/%s/%d: %w", deployment, service, replica, err)
	}

	for port := HealthPortMin; port <= HealthPortMax; port++ {
		_, err = tx.Exec(`
			INSERT INTO health_endpoints(customer_id, service_name, replica_index, host_port)
			VALUES(?, ?, ?, ?)
		`, deployment, service, replica, port)
		if err == nil {
			endpoint.HostPort = port
			if err := tx.Commit(); err != nil {
				return HealthEndpoint{}, fmt.Errorf("commit health endpoint reservation: %w", err)
			}
			return endpoint, nil
		}
	}
	return HealthEndpoint{}, fmt.Errorf("health endpoint port range %d-%d is exhausted", HealthPortMin, HealthPortMax)
}

// ListHealthEndpoints returns reserved endpoints for a deployment in stable
// service/replica order. A deployment created before health support returns an
// empty list.
func (s *Store) ListHealthEndpoints(deployment string) ([]HealthEndpoint, error) {
	rows, err := s.db.Query(`
		SELECT customer_id, service_name, replica_index, host_port
		FROM health_endpoints
		WHERE customer_id = ?
		ORDER BY service_name, replica_index
	`, deployment)
	if err != nil {
		return nil, fmt.Errorf("list health endpoints for %s: %w", deployment, err)
	}
	defer rows.Close()

	var endpoints []HealthEndpoint
	for rows.Next() {
		var endpoint HealthEndpoint
		if err := rows.Scan(&endpoint.Deployment, &endpoint.Service, &endpoint.Replica, &endpoint.HostPort); err != nil {
			return nil, fmt.Errorf("scan health endpoint: %w", err)
		}
		endpoints = append(endpoints, endpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate health endpoints: %w", err)
	}
	return endpoints, nil
}

// DeleteHealthEndpoints releases all health endpoint reservations for a
// deployment after its compose projects have been torn down.
func (s *Store) DeleteHealthEndpoints(deployment string) error {
	if _, err := s.db.Exec(`DELETE FROM health_endpoints WHERE customer_id = ?`, deployment); err != nil {
		return fmt.Errorf("delete health endpoints for %s: %w", deployment, err)
	}
	return nil
}

// AllocateWirelessGroup returns an existing allocation or reserves the lowest
// free bit. Store operations are serialized by the single SQLite connection.
func (s *Store) AllocateWirelessGroup(deployment string) (WirelessGroup, error) {
	if deployment == "" {
		return WirelessGroup{}, fmt.Errorf("wireless group deployment is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return WirelessGroup{}, fmt.Errorf("begin wireless group allocation: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	group := WirelessGroup{Deployment: deployment}
	err = tx.QueryRow(`SELECT bit_index FROM wireless_groups WHERE customer_id = ?`, deployment).Scan(&group.Bit)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return WirelessGroup{}, fmt.Errorf("commit existing wireless group: %w", err)
		}
		return group, nil
	}
	if err != sql.ErrNoRows {
		return WirelessGroup{}, fmt.Errorf("lookup wireless group for %s: %w", deployment, err)
	}

	used := map[int]struct{}{}
	rows, err := tx.Query(`SELECT bit_index FROM wireless_groups`)
	if err != nil {
		return WirelessGroup{}, fmt.Errorf("list wireless group allocations: %w", err)
	}
	for rows.Next() {
		var bit int
		if err := rows.Scan(&bit); err != nil {
			rows.Close()
			return WirelessGroup{}, fmt.Errorf("scan wireless group allocation: %w", err)
		}
		used[bit] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return WirelessGroup{}, fmt.Errorf("iterate wireless group allocations: %w", err)
	}
	if err := rows.Close(); err != nil {
		return WirelessGroup{}, fmt.Errorf("close wireless group rows: %w", err)
	}
	for bit := 0; bit < 64; bit++ {
		if _, exists := used[bit]; exists {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO wireless_groups(customer_id, bit_index, updated_at) VALUES(?, ?, ?)`, deployment, bit, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return WirelessGroup{}, fmt.Errorf("reserve wireless group bit %d for %s: %w", bit, deployment, err)
		}
		group.Bit = bit
		if err := tx.Commit(); err != nil {
			return WirelessGroup{}, fmt.Errorf("commit wireless group allocation: %w", err)
		}
		return group, nil
	}
	return WirelessGroup{}, fmt.Errorf("wireless group capacity exhausted: all 64 bits are allocated")
}

func (s *Store) WirelessGroup(deployment string) (WirelessGroup, bool, error) {
	group := WirelessGroup{Deployment: deployment}
	err := s.db.QueryRow(`SELECT bit_index FROM wireless_groups WHERE customer_id = ?`, deployment).Scan(&group.Bit)
	if err == sql.ErrNoRows {
		return WirelessGroup{}, false, nil
	}
	if err != nil {
		return WirelessGroup{}, false, fmt.Errorf("query wireless group for %s: %w", deployment, err)
	}
	return group, true, nil
}

func (s *Store) ReleaseWirelessGroup(deployment string) error {
	var radios int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM wireless_radios WHERE customer_id = ?`, deployment).Scan(&radios); err != nil {
		return fmt.Errorf("count radios before releasing group for %s: %w", deployment, err)
	}
	if radios != 0 {
		return fmt.Errorf("cannot release wireless group for %s while %d radio records remain", deployment, radios)
	}
	if _, err := s.db.Exec(`DELETE FROM wireless_groups WHERE customer_id = ?`, deployment); err != nil {
		return fmt.Errorf("release wireless group for %s: %w", deployment, err)
	}
	return nil
}

// ReplaceWirelessRadios atomically replaces one deployment's desired records.
func (s *Store) ReplaceWirelessRadios(deployment string, radios []WirelessRadio) (err error) {
	group, ok, err := s.WirelessGroup(deployment)
	if err != nil {
		return err
	}
	if len(radios) > 0 && !ok {
		return fmt.Errorf("wireless group for %s is not allocated", deployment)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin wireless radio replace: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.Exec(`DELETE FROM wireless_radios WHERE customer_id = ?`, deployment); err != nil {
		return fmt.Errorf("delete wireless radios for %s: %w", deployment, err)
	}
	for _, radio := range radios {
		if radio.Deployment != deployment || radio.Service == "" || radio.Replica < 0 || radio.LogicalName == "" || radio.ManagerName == "" || radio.GroupBit != group.Bit {
			return fmt.Errorf("invalid wireless radio identity for deployment %s: %+v", deployment, radio)
		}
		_, err = tx.Exec(`
			INSERT INTO wireless_radios(customer_id, service_name, replica_index, logical_name, manager_name, mac, network_name, device_name, mode, bridge_name, container_name, group_bit, status, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, radio.Deployment, radio.Service, radio.Replica, radio.LogicalName, radio.ManagerName, radio.MAC, radio.Network, radio.Device, radio.Mode, radio.Bridge, radio.ContainerName, radio.GroupBit, radio.Status, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return fmt.Errorf("insert wireless radio %s: %w", radio.ManagerName, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit wireless radio replace: %w", err)
	}
	return nil
}

func (s *Store) ListWirelessRadios(deployment string) ([]WirelessRadio, error) {
	query := `SELECT customer_id, service_name, replica_index, logical_name, manager_name, mac, network_name, device_name, mode, bridge_name, container_name, group_bit, status FROM wireless_radios`
	args := []any{}
	if deployment != "" {
		query += ` WHERE customer_id = ?`
		args = append(args, deployment)
	}
	query += ` ORDER BY customer_id, service_name, replica_index, logical_name`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list wireless radios: %w", err)
	}
	defer rows.Close()
	var radios []WirelessRadio
	for rows.Next() {
		var radio WirelessRadio
		if err := rows.Scan(&radio.Deployment, &radio.Service, &radio.Replica, &radio.LogicalName, &radio.ManagerName, &radio.MAC, &radio.Network, &radio.Device, &radio.Mode, &radio.Bridge, &radio.ContainerName, &radio.GroupBit, &radio.Status); err != nil {
			return nil, fmt.Errorf("scan wireless radio: %w", err)
		}
		radios = append(radios, radio)
	}
	return radios, rows.Err()
}

func (s *Store) DeleteWirelessRadios(deployment string) error {
	if _, err := s.db.Exec(`DELETE FROM wireless_radios WHERE customer_id = ?`, deployment); err != nil {
		return fmt.Errorf("delete wireless radios for %s: %w", deployment, err)
	}
	return nil
}

func (s *Store) UpdateWirelessRadioStatus(deployment, managerName, status string) error {
	result, err := s.db.Exec(`UPDATE wireless_radios SET status = ?, updated_at = ? WHERE customer_id = ? AND manager_name = ?`, status, time.Now().UTC().Format(time.RFC3339Nano), deployment, managerName)
	if err != nil {
		return fmt.Errorf("update wireless radio %s status: %w", managerName, err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return fmt.Errorf("wireless radio %s is not owned by deployment %s", managerName, deployment)
	}
	return nil
}

func (s *Store) WirelessKeepSet() ([]string, error) {
	rows, err := s.db.Query(`SELECT manager_name FROM wireless_radios ORDER BY manager_name`)
	if err != nil {
		return nil, fmt.Errorf("query wireless keep set: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan wireless keep name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
