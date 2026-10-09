package persist

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

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

func TestWirelessGroupAllocationIsStableLowestFreeAndExhaustible(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	first, err := store.AllocateWirelessGroup("edge")
	if err != nil || first.Bit != 0 || first.Mask() != 1 {
		t.Fatalf("first allocation = %+v, %v", first, err)
	}
	again, err := store.AllocateWirelessGroup("edge")
	if err != nil || again != first {
		t.Fatalf("stable allocation = %+v, %v; want %+v", again, err, first)
	}
	for bit := 1; bit < 64; bit++ {
		group, err := store.AllocateWirelessGroup(fmt.Sprintf("edge-%d", bit))
		if err != nil || group.Bit != bit {
			t.Fatalf("allocation %d = %+v, %v", bit, group, err)
		}
	}
	if _, err := store.AllocateWirelessGroup("overflow"); err == nil || !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestConcurrentWirelessGroupAllocationIsUnique(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	const count = 16
	bits := make(chan int, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			group, err := store.AllocateWirelessGroup(fmt.Sprintf("deployment-%d", index))
			if err != nil {
				errs <- err
				return
			}
			bits <- group.Bit
		}(index)
	}
	wg.Wait()
	close(bits)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent allocation: %v", err)
	}
	var got []int
	for bit := range bits {
		got = append(got, bit)
	}
	sort.Ints(got)
	for bit := 0; bit < count; bit++ {
		if got[bit] != bit {
			t.Fatalf("allocated bits = %v", got)
		}
	}
}

func TestWirelessRadioReplacementIsAtomicAndBuildsGlobalKeepSet(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	for _, deployment := range []string{"edge-a", "edge-b"} {
		group, err := store.AllocateWirelessGroup(deployment)
		if err != nil {
			t.Fatalf("allocate %s: %v", deployment, err)
		}
		radio := WirelessRadio{Deployment: deployment, Service: "gateway", Replica: 0, LogicalName: "ap", ManagerName: "vcpe-" + deployment, MAC: "02:00:00:00:00:01", Network: "home", Device: "wlan0", Mode: "ap", Bridge: "brlan0", ContainerName: deployment + "-gateway-1", GroupBit: group.Bit, Status: "planned"}
		if err := store.ReplaceWirelessRadios(deployment, []WirelessRadio{radio}); err != nil {
			t.Fatalf("replace %s: %v", deployment, err)
		}
	}
	keep, err := store.WirelessKeepSet()
	if err != nil || !reflect.DeepEqual(keep, []string{"vcpe-edge-a", "vcpe-edge-b"}) {
		t.Fatalf("keep set = %v, %v", keep, err)
	}

	group, _, _ := store.WirelessGroup("edge-a")
	invalid := []WirelessRadio{
		{Deployment: "edge-a", Service: "gateway", LogicalName: "one", ManagerName: "duplicate", GroupBit: group.Bit},
		{Deployment: "edge-a", Service: "gateway", LogicalName: "two", ManagerName: "duplicate", GroupBit: group.Bit},
	}
	if err := store.ReplaceWirelessRadios("edge-a", invalid); err == nil {
		t.Fatal("expected duplicate manager name to fail")
	}
	radios, err := store.ListWirelessRadios("edge-a")
	if err != nil || len(radios) != 1 || radios[0].ManagerName != "vcpe-edge-a" {
		t.Fatalf("atomic rollback radios = %+v, %v", radios, err)
	}
	if err := store.ReleaseWirelessGroup("edge-a"); err == nil || !strings.Contains(err.Error(), "remain") {
		t.Fatalf("release with radios error = %v", err)
	}
	if err := store.DeleteWirelessRadios("edge-a"); err != nil {
		t.Fatalf("delete radios: %v", err)
	}
	if err := store.ReleaseWirelessGroup("edge-a"); err != nil {
		t.Fatalf("release group: %v", err)
	}
}

func TestResetClearsWirelessOwnershipAndRestampsV3(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	if SchemaVersion != "vcpe.dev/state/v3" {
		t.Fatalf("SchemaVersion = %q", SchemaVersion)
	}
	var version string
	if err := store.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("fresh schema stamp = %q, %v", version, err)
	}
	if _, err := store.AllocateWirelessGroup("edge"); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if _, err := store.db.Exec(`UPDATE meta SET value = 'vcpe.dev/state/v2' WHERE key = 'schema_version'`); err != nil {
		t.Fatalf("set old schema stamp: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close old store: %v", err)
	}
	if _, err := Open(root); err == nil || !strings.Contains(err.Error(), "vcpe.dev/state/v2") || !strings.Contains(err.Error(), "vcpe state reset") {
		t.Fatalf("Open() incompatible error = %v", err)
	}
	store, err = OpenForReset(root)
	if err != nil {
		t.Fatalf("OpenForReset() error = %v", err)
	}
	defer store.Close()
	if err := store.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if err := store.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("reset schema stamp = %q, %v", version, err)
	}
	if _, ok, err := store.WirelessGroup("edge"); err != nil || ok {
		t.Fatalf("group after reset: ok=%t err=%v", ok, err)
	}
	metrics, err := store.Metrics()
	if err != nil || metrics.WirelessGroups != 0 || metrics.WirelessRadios != 0 {
		t.Fatalf("metrics after reset = %+v, %v", metrics, err)
	}
}
