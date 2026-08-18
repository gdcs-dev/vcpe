// Package diagnosticstate stores bounded in-memory evidence for webhook
// diagnostics. It deliberately contains no credential, secret, or payload data.
package diagnosticstate

import (
	"net/url"
	"regexp"
	"sync"
	"time"
)

const (
	DefaultMaxReceipts = 64
	DefaultReceiptTTL  = 10 * time.Minute
	SourceDirect       = "direct"
	SourceCaduceus     = "caduceus"
)

var (
	stableIDPattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	correlationIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Intent is the non-sensitive subscriber-owned webhook configuration.
type Intent struct {
	CallbackURL      string
	EventFilter      string
	DeviceMatcher    string
	ContentType      string
	SecretConfigured bool
}

// Receipt records a successfully accepted diagnostic callback.
type Receipt struct {
	CorrelationID string
	Source        string
	AcceptedAt    time.Time
	HTTPStatus    int
}

// Snapshot is an output-safe copy of the current diagnostic state.
type Snapshot struct {
	Intent            Intent
	ObservedAt        time.Time
	InitialSuccessAt  time.Time
	LastFailureAt     time.Time
	RefreshSuccessAt  time.Time
	RefreshFailureAt  time.Time
	LastErrorCategory string
}

// Config controls bounded receipt retention. Zero values use safe defaults.
type Config struct {
	MaxReceipts int
	ReceiptTTL  time.Duration
	Now         func() time.Time
}

// Store owns concurrent diagnostic state for one event-sink process.
type Store struct {
	mu          sync.Mutex
	now         func() time.Time
	maxReceipts int
	receiptTTL  time.Duration
	snapshot    Snapshot
	receipts    map[string]Receipt
	order       []string
}

// New creates an empty in-memory store with the configured registration intent.
func New(intent Intent, config Config) *Store {
	if config.MaxReceipts <= 0 {
		config.MaxReceipts = DefaultMaxReceipts
	}
	if config.ReceiptTTL <= 0 {
		config.ReceiptTTL = DefaultReceiptTTL
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Store{
		now:         config.Now,
		maxReceipts: config.MaxReceipts,
		receiptTTL:  config.ReceiptTTL,
		snapshot:    Snapshot{Intent: sanitizeIntent(intent)},
		receipts:    make(map[string]Receipt),
	}
}

func sanitizeIntent(intent Intent) Intent {
	parsed, err := url.Parse(intent.CallbackURL)
	if err != nil {
		intent.CallbackURL = ""
		return intent
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	intent.CallbackURL = parsed.String()
	return intent
}

// Snapshot returns the current output-safe registration evidence.
func (store *Store) Snapshot() Snapshot {
	store.mu.Lock()
	defer store.mu.Unlock()
	snapshot := store.snapshot
	snapshot.ObservedAt = store.timestamp(time.Time{})
	return snapshot
}

// RecordInitialSuccess stores the first completed Argus registration time.
func (store *Store) RecordInitialSuccess(at time.Time) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.snapshot.InitialSuccessAt = store.timestamp(at)
	store.snapshot.LastErrorCategory = ""
}

// RecordInitialFailure stores the latest initial-registration failure without
// affecting the registration retry behavior.
func (store *Store) RecordInitialFailure(at time.Time, category string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.recordFailureLocked(at, category)
}

// RecordRefreshSuccess stores the latest completed refresh time.
func (store *Store) RecordRefreshSuccess(at time.Time) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.snapshot.RefreshSuccessAt = store.timestamp(at)
	store.snapshot.LastErrorCategory = ""
}

// RecordRefreshFailure stores only a normalized, non-sensitive error category.
func (store *Store) RecordRefreshFailure(at time.Time, category string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.snapshot.RefreshFailureAt = store.timestamp(at)
	store.recordFailureLocked(at, category)
}

func (store *Store) recordFailureLocked(at time.Time, category string) {
	store.snapshot.LastFailureAt = store.timestamp(at)
	if stableIDPattern.MatchString(category) {
		store.snapshot.LastErrorCategory = category
	} else {
		store.snapshot.LastErrorCategory = "registration-failed"
	}
}

// RecordReceipt stores one accepted diagnostic callback. It returns false for
// invalid receipt metadata and otherwise keeps the latest receipt by ID.
func (store *Store) RecordReceipt(receipt Receipt) bool {
	if !ValidCorrelationID(receipt.CorrelationID) || (receipt.Source != SourceDirect && receipt.Source != SourceCaduceus) || receipt.HTTPStatus < 100 || receipt.HTTPStatus > 599 {
		return false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.evictExpiredLocked()
	receipt.AcceptedAt = store.timestamp(receipt.AcceptedAt)
	if _, exists := store.receipts[receipt.CorrelationID]; !exists {
		store.order = append(store.order, receipt.CorrelationID)
	}
	store.receipts[receipt.CorrelationID] = receipt
	for len(store.order) > store.maxReceipts {
		delete(store.receipts, store.order[0])
		store.order = store.order[1:]
	}
	return true
}

// Receipt returns a non-expired receipt by correlation ID.
func (store *Store) Receipt(correlationID string) (Receipt, bool) {
	if !ValidCorrelationID(correlationID) {
		return Receipt{}, false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.evictExpiredLocked()
	receipt, ok := store.receipts[correlationID]
	return receipt, ok
}

// ValidCorrelationID accepts only the opaque bounded ID shared by the CPE,
// WebPA, and subscriber diagnostic contracts.
func ValidCorrelationID(value string) bool { return correlationIDPattern.MatchString(value) }

func (store *Store) timestamp(value time.Time) time.Time {
	if value.IsZero() {
		value = store.now()
	}
	return value.UTC()
}

func (store *Store) evictExpiredLocked() {
	now := store.now()
	retained := store.order[:0]
	for _, correlationID := range store.order {
		receipt := store.receipts[correlationID]
		if now.Sub(receipt.AcceptedAt) >= store.receiptTTL {
			delete(store.receipts, correlationID)
			continue
		}
		retained = append(retained, correlationID)
	}
	store.order = retained
}
