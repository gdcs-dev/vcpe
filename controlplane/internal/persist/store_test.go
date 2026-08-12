package persist

import "testing"

func TestHealthEndpointReservationsAreStableAndReleased(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	first, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatalf("ReserveHealthEndpoint() error = %v", err)
	}
	if first.HostPort != HealthPortMin {
		t.Fatalf("first HostPort = %d, want %d", first.HostPort, HealthPortMin)
	}

	again, err := store.ReserveHealthEndpoint("edge", "gateway", 0)
	if err != nil {
		t.Fatalf("reserve existing endpoint error = %v", err)
	}
	if again.HostPort != first.HostPort {
		t.Fatalf("existing HostPort = %d, want %d", again.HostPort, first.HostPort)
	}

	second, err := store.ReserveHealthEndpoint("edge", "webpa", 0)
	if err != nil {
		t.Fatalf("reserve second endpoint error = %v", err)
	}
	if second.HostPort == first.HostPort {
		t.Fatal("different endpoints received the same host port")
	}

	endpoints, err := store.ListHealthEndpoints("edge")
	if err != nil {
		t.Fatalf("ListHealthEndpoints() error = %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("endpoint count = %d, want 2", len(endpoints))
	}

	if err := store.DeleteHealthEndpoints("edge"); err != nil {
		t.Fatalf("DeleteHealthEndpoints() error = %v", err)
	}
	endpoints, err = store.ListHealthEndpoints("edge")
	if err != nil {
		t.Fatalf("ListHealthEndpoints() after delete error = %v", err)
	}
	if len(endpoints) != 0 {
		t.Fatalf("endpoint count after delete = %d, want 0", len(endpoints))
	}
}
