package diagnosticstate

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreTracksRegistrationTransitions(t *testing.T) {
	now := time.Date(2026, time.August, 14, 19, 0, 0, 0, time.UTC)
	store := New(Intent{CallbackURL: "http://event-sink:8080/webhook", EventFilter: "apparmor/.*", DeviceMatcher: ".*", ContentType: "application/json"}, Config{Now: func() time.Time { return now }})
	store.RecordInitialSuccess(time.Time{})
	store.RecordRefreshFailure(now.Add(time.Minute), "argus-authentication-failed")
	store.RecordRefreshSuccess(now.Add(2 * time.Minute))

	snapshot := store.Snapshot()
	if snapshot.Intent.CallbackURL != "http://event-sink:8080/webhook" || snapshot.InitialSuccessAt != now || snapshot.RefreshSuccessAt != now.Add(2*time.Minute) || !snapshot.RefreshFailureAt.Equal(now.Add(time.Minute)) || snapshot.LastErrorCategory != "" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestStoreBoundsAndExpiresReceipts(t *testing.T) {
	now := time.Date(2026, time.August, 14, 19, 0, 0, 0, time.UTC)
	store := New(Intent{}, Config{MaxReceipts: 2, ReceiptTTL: time.Minute, Now: func() time.Time { return now }})
	first := strings.Repeat("a", 64)
	second := strings.Repeat("b", 64)
	third := strings.Repeat("c", 64)
	for _, receipt := range []Receipt{
		{CorrelationID: first, Source: SourceDirect, HTTPStatus: 204},
		{CorrelationID: second, Source: SourceCaduceus, HTTPStatus: 204},
		{CorrelationID: third, Source: SourceDirect, HTTPStatus: 204},
	} {
		if !store.RecordReceipt(receipt) {
			t.Fatalf("RecordReceipt(%+v) = false", receipt)
		}
	}
	if _, ok := store.Receipt(first); ok {
		t.Fatal("oldest receipt was not evicted")
	}
	now = now.Add(time.Minute)
	if _, ok := store.Receipt(second); ok {
		t.Fatal("expired receipt was retained")
	}
}

func TestStoreRejectsUnsafeReceiptMetadata(t *testing.T) {
	store := New(Intent{}, Config{})
	if store.RecordReceipt(Receipt{CorrelationID: "not/a-correlation", Source: SourceDirect, HTTPStatus: 204}) {
		t.Fatal("unsafe correlation ID was accepted")
	}
	if store.RecordReceipt(Receipt{CorrelationID: strings.Repeat("a", 64), Source: "other", HTTPStatus: 204}) {
		t.Fatal("unknown source was accepted")
	}
	if store.RecordReceipt(Receipt{CorrelationID: strings.Repeat("a", 64), Source: SourceDirect, HTTPStatus: 99}) {
		t.Fatal("invalid HTTP status was accepted")
	}
}

func TestStoreConcurrentTransitionsAndRestartEmpty(t *testing.T) {
	store := New(Intent{}, Config{})
	var group sync.WaitGroup
	for index := 0; index < 32; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			store.RecordInitialSuccess(time.Time{})
			store.RecordRefreshFailure(time.Time{}, "argus-registration-failed")
			store.RecordRefreshSuccess(time.Time{})
			store.RecordReceipt(Receipt{CorrelationID: strings.Repeat(string(rune('a'+index%26)), 64), Source: SourceDirect, HTTPStatus: 204})
			_ = store.Snapshot()
		}(index)
	}
	group.Wait()
	if store.Snapshot().InitialSuccessAt.IsZero() {
		t.Fatal("concurrent transitions lost initial success")
	}
	restarted := New(Intent{}, Config{})
	if snapshot := restarted.Snapshot(); !snapshot.InitialSuccessAt.IsZero() || snapshot.LastErrorCategory != "" {
		t.Fatalf("restarted snapshot = %+v, want empty state", snapshot)
	}
	if _, ok := restarted.Receipt(strings.Repeat("a", 64)); ok {
		t.Fatal("restarted store retained a receipt")
	}
}
