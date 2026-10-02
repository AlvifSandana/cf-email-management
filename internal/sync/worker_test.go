package sync

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
	"github.com/bariskode/email-management-service/internal/provider/mock"
	"github.com/bariskode/email-management-service/internal/storage"
	"github.com/bariskode/email-management-service/internal/storage/memory"
)

func setupTestWorkerEnvironment() (*storage.Repositories, *mock.Provider, *Engine, *slog.Logger) {
	repos := memory.New()
	prov := mock.New()
	auditService := audit.NewService(repos.Audit)
	engine := NewEngine(repos, prov, auditService)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return repos, prov, engine, logger
}

func TestWorker_StartGracefulTermination(t *testing.T) {
	repos, _, engine, logger := setupTestWorkerEnvironment()

	ctx, cancel := context.WithCancel(context.Background())
	worker := NewWorker(repos, engine, 10*time.Millisecond, logger, nil)

	stopped := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(stopped)
	}()

	// Let it run briefly
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-stopped:
		// Succeeded gracefully
	case <-time.After(1 * time.Second):
		t.Fatal("worker did not terminate gracefully after ctx.Done()")
	}
}

func TestWorker_InitialImmediateTick(t *testing.T) {
	repos, prov, engine, logger := setupTestWorkerEnvironment()
	ctx := context.Background()

	zoneID := "zon_init_test"
	providerZoneID := "cf_init_test"

	_ = repos.Zones.Save(ctx, &domain.Zone{
		ID:                  zoneID,
		ProviderZoneID:      providerZoneID,
		Name:                "initial.test",
		Status:              "active",
		EmailRoutingEnabled: true,
	})

	// Setup remote drift: rule exists remotely but not locally
	prov.Settings[providerZoneID] = provider.RoutingSettings{Enabled: true}
	prov.Rules[providerZoneID] = map[string]domain.RoutingRule{
		"cf_r1": {
			ProviderRuleID: "cf_r1",
			Name:           "Remote Only",
			MatcherType:    "literal",
			MatcherField:   "to",
			MatcherValue:   "remote@initial.test",
			ActionType:     "forward",
			Destination:    "dest@gmail.com",
			Enabled:        true,
		},
	}

	driftTriggered := make(chan struct{}, 1)
	onDrift := func(zone domain.Zone, result *SyncResult) {
		if zone.ID == zoneID && len(result.Diffs) > 0 {
			driftTriggered <- struct{}{}
		}
	}

	// Large interval (1 hour) — if initial tick doesn't run immediately, test times out
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	worker := NewWorker(repos, engine, 1*time.Hour, logger, onDrift)
	go worker.Start(workerCtx)

	select {
	case <-driftTriggered:
		// Initial immediate tick executed successfully
	case <-time.After(1 * time.Second):
		t.Fatal("expected initial immediate tick to run, but onDrift was not called")
	}
}

func TestWorker_OnDriftFuncCalledWithDrift(t *testing.T) {
	repos, prov, engine, logger := setupTestWorkerEnvironment()
	ctx := context.Background()

	zoneID := "zon_drift_cb"
	providerZoneID := "cf_drift_cb"

	zone := &domain.Zone{
		ID:                  zoneID,
		ProviderZoneID:      providerZoneID,
		Name:                "callback.test",
		Status:              "active",
		EmailRoutingEnabled: true,
	}
	_ = repos.Zones.Save(ctx, zone)

	// Local rule
	_ = repos.Rules.Save(ctx, &domain.RoutingRule{
		ID:             "rul_local_1",
		ZoneID:         zoneID,
		ProviderRuleID: "cf_r1",
		Name:           "Hello",
		MatcherType:    "literal",
		MatcherField:   "to",
		MatcherValue:   "hello@callback.test",
		ActionType:     "forward",
		Destination:    "original@dest.com",
		Enabled:        true,
	})

	// Remote has modified destination (CHANGED)
	prov.Settings[providerZoneID] = provider.RoutingSettings{Enabled: true}
	prov.Rules[providerZoneID] = map[string]domain.RoutingRule{
		"cf_r1": {
			ProviderRuleID: "cf_r1",
			Name:           "Hello",
			MatcherType:    "literal",
			MatcherField:   "to",
			MatcherValue:   "hello@callback.test",
			ActionType:     "forward",
			Destination:    "changed@dest.com",
			Enabled:        true,
		},
	}

	var calledWithZone domain.Zone
	var calledWithResult *SyncResult
	var callCount int32

	onDrift := func(z domain.Zone, result *SyncResult) {
		atomic.AddInt32(&callCount, 1)
		calledWithZone = z
		calledWithResult = result
	}

	worker := NewWorker(repos, engine, 10*time.Millisecond, logger, onDrift)
	err := worker.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Fatalf("expected onDrift to be called exactly 1 time, got %d", atomic.LoadInt32(&callCount))
	}
	if calledWithZone.ID != zoneID {
		t.Errorf("expected zone id %s, got %s", zoneID, calledWithZone.ID)
	}
	if calledWithResult == nil || len(calledWithResult.Diffs) == 0 {
		t.Fatal("expected non-empty diffs in SyncResult")
	}

	foundChanged := false
	for _, diff := range calledWithResult.Diffs {
		if diff.Identifier == "hello@callback.test" && diff.Status == domain.DiffStatusChanged {
			foundChanged = true
		}
	}
	if !foundChanged {
		t.Errorf("expected changed diff for hello@callback.test")
	}
}

func TestWorker_NoDrift_OnDriftFuncNotCalled(t *testing.T) {
	repos, prov, engine, logger := setupTestWorkerEnvironment()
	ctx := context.Background()

	zoneID := "zon_in_sync"
	providerZoneID := "cf_in_sync"

	_ = repos.Zones.Save(ctx, &domain.Zone{
		ID:                  zoneID,
		ProviderZoneID:      providerZoneID,
		Name:                "insync.test",
		Status:              "active",
		EmailRoutingEnabled: true,
	})

	_ = repos.Rules.Save(ctx, &domain.RoutingRule{
		ID:             "rul_1",
		ZoneID:         zoneID,
		ProviderRuleID: "cf_r1",
		Name:           "Rule 1",
		MatcherType:    "literal",
		MatcherField:   "to",
		MatcherValue:   "r1@insync.test",
		ActionType:     "forward",
		Destination:    "dest@test.com",
		Enabled:        true,
	})

	prov.Settings[providerZoneID] = provider.RoutingSettings{Enabled: true}
	prov.Rules[providerZoneID] = map[string]domain.RoutingRule{
		"cf_r1": {
			ProviderRuleID: "cf_r1",
			Name:           "Rule 1",
			MatcherType:    "literal",
			MatcherField:   "to",
			MatcherValue:   "r1@insync.test",
			ActionType:     "forward",
			Destination:    "dest@test.com",
			Enabled:        true,
		},
	}

	var callCount int32
	onDrift := func(z domain.Zone, result *SyncResult) {
		atomic.AddInt32(&callCount, 1)
	}

	worker := NewWorker(repos, engine, 10*time.Millisecond, logger, onDrift)
	err := worker.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 0 {
		t.Errorf("expected onDrift NOT to be called when states match, got %d calls", atomic.LoadInt32(&callCount))
	}
}

func TestWorker_PeriodicDriftCheck(t *testing.T) {
	repos, prov, engine, logger := setupTestWorkerEnvironment()
	ctx := context.Background()

	zoneID := "zon_periodic"
	providerZoneID := "cf_periodic"

	_ = repos.Zones.Save(ctx, &domain.Zone{
		ID:                  zoneID,
		ProviderZoneID:      providerZoneID,
		Name:                "periodic.test",
		Status:              "active",
		EmailRoutingEnabled: false,
	})

	// Remote setting differs (true vs false) -> Drift!
	prov.Settings[providerZoneID] = provider.RoutingSettings{Enabled: true}

	var callCount int32
	onDrift := func(z domain.Zone, result *SyncResult) {
		atomic.AddInt32(&callCount, 1)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// Short interval of 20ms
	worker := NewWorker(repos, engine, 20*time.Millisecond, logger, onDrift)
	go worker.Start(workerCtx)

	// Wait enough time for initial tick + at least 2 interval ticks (total >= 3)
	time.Sleep(80 * time.Millisecond)
	workerCancel()

	count := atomic.LoadInt32(&callCount)
	if count < 3 {
		t.Errorf("expected at least 3 periodic drift checks, got %d", count)
	}
}

func TestWorker_OnDriftFuncNil(t *testing.T) {
	repos, prov, engine, logger := setupTestWorkerEnvironment()
	ctx := context.Background()

	zoneID := "zon_nil_cb"
	providerZoneID := "cf_nil_cb"

	_ = repos.Zones.Save(ctx, &domain.Zone{
		ID:                  zoneID,
		ProviderZoneID:      providerZoneID,
		Name:                "nilcb.test",
		Status:              "active",
		EmailRoutingEnabled: false,
	})

	prov.Settings[providerZoneID] = provider.RoutingSettings{Enabled: true}

	// onDriftFunc is nil; must not panic
	worker := NewWorker(repos, engine, 10*time.Millisecond, logger, nil)
	if err := worker.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile failed with nil onDriftFunc: %v", err)
	}
}

func TestWorker_SyncZoneError_ContinuesOtherZones(t *testing.T) {
	repos, prov, engine, logger := setupTestWorkerEnvironment()
	ctx := context.Background()

	// Zone 1: Locked
	zone1 := &domain.Zone{
		ID:                  "zon_err_1",
		ProviderZoneID:      "cf_err_1",
		Name:                "locked.test",
		Status:              "active",
		EmailRoutingEnabled: true,
	}
	_ = repos.Zones.Save(ctx, zone1)
	lock := engine.getZoneLock("zon_err_1")
	lock.Lock() // artificially lock zone 1 so SyncZone returns ErrZoneSyncLocked
	defer lock.Unlock()

	// Zone 2: Healthy, with drift
	zone2 := &domain.Zone{
		ID:                  "zon_ok_2",
		ProviderZoneID:      "cf_ok_2",
		Name:                "healthy.test",
		Status:              "active",
		EmailRoutingEnabled: false,
	}
	_ = repos.Zones.Save(ctx, zone2)
	prov.Settings["cf_ok_2"] = provider.RoutingSettings{Enabled: true}

	var driftedZones []string
	var mu sync.Mutex
	onDrift := func(z domain.Zone, result *SyncResult) {
		mu.Lock()
		driftedZones = append(driftedZones, z.ID)
		mu.Unlock()
	}

	worker := NewWorker(repos, engine, 10*time.Millisecond, logger, onDrift)
	err := worker.Reconcile(ctx)
	if err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(driftedZones) != 1 || driftedZones[0] != "zon_ok_2" {
		t.Errorf("expected zone 2 to be processed despite zone 1 error, got %v", driftedZones)
	}
}

func TestWorker_EmptyZones(t *testing.T) {
	repos, _, engine, logger := setupTestWorkerEnvironment()
	ctx := context.Background()

	worker := NewWorker(repos, engine, 10*time.Millisecond, logger, nil)
	if err := worker.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile with 0 zones should succeed, got %v", err)
	}
}

func TestWorker_AlreadyCancelledContext(t *testing.T) {
	repos, _, engine, logger := setupTestWorkerEnvironment()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	worker := NewWorker(repos, engine, 10*time.Millisecond, logger, nil)

	stopped := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(stopped)
	}()

	select {
	case <-stopped:
		// Succeeded immediately
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker should terminate immediately when given an already cancelled context")
	}
}

func TestWorker_DefaultsAndGetters(t *testing.T) {
	repos, _, engine, _ := setupTestWorkerEnvironment()

	// Pass nil logger and negative interval
	worker := NewWorker(repos, engine, -1, nil, nil)

	if worker.Interval() != 5*time.Minute {
		t.Errorf("expected default 5m interval, got %v", worker.Interval())
	}
	if worker.Logger() == nil {
		t.Errorf("expected non-nil default logger")
	}
	if worker.Repos() != repos {
		t.Errorf("expected repos to match")
	}
	if worker.Engine() != engine {
		t.Errorf("expected engine to match")
	}
}
