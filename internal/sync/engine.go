package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/bariskode/email-management-service/internal/audit"
	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/provider"
	"github.com/bariskode/email-management-service/internal/storage"
)

// Engine orchestrates synchronization and drift detection.
type Engine struct {
	repos     *storage.Repositories
	provider  provider.EmailProvider
	audit     *audit.Service
	zoneLocks sync.Map // map[string]*sync.Mutex for per-zone concurrency control
}

// NewEngine creates a new Sync Engine.
func NewEngine(repos *storage.Repositories, prov provider.EmailProvider, audit *audit.Service) *Engine {
	return &Engine{
		repos:    repos,
		provider: prov,
		audit:    audit,
	}
}

func (e *Engine) getZoneLock(zoneID string) *sync.Mutex {
	lock, _ := e.zoneLocks.LoadOrStore(zoneID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// SyncResult details the outcome of a synchronization run.
type SyncResult struct {
	RunID        string            `json:"run_id"`
	ZoneID       string            `json:"zone_id"`
	Direction    string            `json:"direction"`
	Status       string            `json:"status"`
	Diffs        []domain.DiffItem `json:"diffs"`
	ChangesCount int               `json:"changes_count"`
	ErrorCount   int               `json:"error_count"`
	Summary      string            `json:"summary"`
}

// SyncZone performs synchronization or drift detection for a specific zone.
// Supported directions: "drift_check", "pull" (remote -> local), "push" (local -> remote).
func (e *Engine) SyncZone(ctx context.Context, zoneID string, direction string, requestID string) (*SyncResult, error) {
	zoneLock := e.getZoneLock(zoneID)
	if !zoneLock.TryLock() {
		return nil, domain.ErrZoneSyncLocked
	}
	defer zoneLock.Unlock()

	startTime := time.Now().UTC()
	runID := generateID("syn")

	zone, err := e.repos.Zones.Get(ctx, zoneID)
	if err != nil {
		return nil, err
	}

	run := &domain.SyncRun{
		ID:        runID,
		ZoneID:    zoneID,
		Direction: direction,
		Status:    "in_progress",
		StartedAt: startTime,
	}
	_ = e.repos.SyncRuns.Save(ctx, run)

	// Step 1: Gather remote state
	remoteSettings, err := e.provider.GetEmailRoutingSettings(ctx, zone.ProviderZoneID)
	if err != nil {
		return e.recordFailure(ctx, run, err, requestID)
	}

	remoteRules, err := e.provider.ListRules(ctx, zone.ProviderZoneID)
	if err != nil {
		return e.recordFailure(ctx, run, err, requestID)
	}

	remoteCatchAll, err := e.provider.GetCatchAll(ctx, zone.ProviderZoneID)
	if err != nil {
		return e.recordFailure(ctx, run, err, requestID)
	}

	// Step 2: Gather local state
	localRules, err := e.repos.Rules.ListByZone(ctx, zoneID)
	if err != nil {
		return e.recordFailure(ctx, run, err, requestID)
	}

	localCatchAll, _ := e.repos.CatchAll.GetByZone(ctx, zoneID)

	// Step 3: Compute diffs
	diffs := computeDiffs(zone, remoteSettings, localRules, remoteRules, localCatchAll, remoteCatchAll)

	changesCount := 0
	errorCount := 0

	// Step 4: Apply reconciliation if direction is pull or push
	switch direction {
	case "pull":
		// Remote -> Local
		// Update zone routing status
		zone.EmailRoutingEnabled = remoteSettings.Enabled
		now := time.Now().UTC()
		zone.LastSyncedAt = &now
		_ = e.repos.Zones.Save(ctx, zone)

		// Synchronize explicit rules
		remoteRuleMap := make(map[string]domain.RoutingRule)
		for _, rr := range remoteRules {
			key := rr.MatcherField + ":" + rr.MatcherValue
			remoteRuleMap[key] = rr
		}

		// Delete local rules that don't exist remotely
		for _, lr := range localRules {
			key := lr.MatcherField + ":" + lr.MatcherValue
			if _, exists := remoteRuleMap[key]; !exists {
				_ = e.repos.Rules.Delete(ctx, lr.ID)
				changesCount++
			}
		}

		// Upsert rules from remote
		for _, rr := range remoteRules {
			var localID string
			for _, lr := range localRules {
				if lr.MatcherField == rr.MatcherField && lr.MatcherValue == rr.MatcherValue {
					localID = lr.ID
					break
				}
			}
			if localID == "" {
				localID = generateID("rul")
			}

			ruleToSave := &domain.RoutingRule{
				ID:             localID,
				ZoneID:         zoneID,
				ProviderRuleID: rr.ProviderRuleID,
				Name:           rr.Name,
				MatcherType:    rr.MatcherType,
				MatcherField:   rr.MatcherField,
				MatcherValue:   rr.MatcherValue,
				ActionType:     rr.ActionType,
				Destination:    rr.Destination,
				Enabled:        rr.Enabled,
				Source:         "sync",
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			_ = e.repos.Rules.Save(ctx, ruleToSave)
			changesCount++
		}

		// Update CatchAll
		caID := generateID("cal")
		if localCatchAll != nil {
			caID = localCatchAll.ID
		}
		caToSave := &domain.CatchAllRule{
			ID:             caID,
			ZoneID:         zoneID,
			ProviderRuleID: remoteCatchAll.ProviderRuleID,
			ActionType:     remoteCatchAll.ActionType,
			Destination:    remoteCatchAll.Destination,
			Enabled:        remoteCatchAll.Enabled,
			Source:         "sync",
			LastSyncedAt:   &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		_ = e.repos.CatchAll.Save(ctx, caToSave)
		changesCount++

	case "push":
		// Local -> Remote
		// Push rules
		for _, lr := range localRules {
			if lr.ProviderRuleID == "" {
				// Create remotely
				created, err := e.provider.CreateRule(ctx, zone.ProviderZoneID, provider.CreateRoutingRule{
					Name:         lr.Name,
					MatcherType:  lr.MatcherType,
					MatcherField: lr.MatcherField,
					MatcherValue: lr.MatcherValue,
					ActionType:   lr.ActionType,
					Destination:  lr.Destination,
					Enabled:      lr.Enabled,
				})
				if err != nil {
					errorCount++
				} else {
					lr.ProviderRuleID = created.ProviderRuleID
					_ = e.repos.Rules.Save(ctx, &lr)
					changesCount++
				}
			} else {
				// Update remotely
				_, err := e.provider.UpdateRule(ctx, zone.ProviderZoneID, lr.ProviderRuleID, provider.UpdateRoutingRule{
					Name:         lr.Name,
					MatcherType:  lr.MatcherType,
					MatcherField: lr.MatcherField,
					MatcherValue: lr.MatcherValue,
					ActionType:   lr.ActionType,
					Destination:  lr.Destination,
					Enabled:      lr.Enabled,
				})
				if err != nil {
					errorCount++
				} else {
					changesCount++
				}
			}
		}

		// Push Catch-all
		if localCatchAll != nil {
			_, err := e.provider.UpdateCatchAll(ctx, zone.ProviderZoneID, provider.UpdateCatchAll{
				ActionType:  localCatchAll.ActionType,
				Destination: localCatchAll.Destination,
				Enabled:     localCatchAll.Enabled,
			})
			if err != nil {
				errorCount++
			} else {
				changesCount++
			}
		}
	}

	finishTime := time.Now().UTC()
	status := "success"
	if errorCount > 0 {
		status = "partial"
	}
	summary := fmt.Sprintf("Completed %s with %d diffs, %d changes, %d errors", direction, len(diffs), changesCount, errorCount)

	run.Status = status
	run.FinishedAt = &finishTime
	run.ChangesCount = changesCount
	run.ErrorCount = errorCount
	run.Summary = summary
	_ = e.repos.SyncRuns.Save(ctx, run)

	_ = e.audit.Record(ctx, audit.RecordParams{
		Operation:    "SYNC_ZONE",
		ResourceType: "zone",
		ResourceID:   zoneID,
		RequestID:    requestID,
		After:        run,
		Status:       status,
	})

	return &SyncResult{
		RunID:        runID,
		ZoneID:       zoneID,
		Direction:    direction,
		Status:       status,
		Diffs:        diffs,
		ChangesCount: changesCount,
		ErrorCount:   errorCount,
		Summary:      summary,
	}, nil
}

func (e *Engine) recordFailure(ctx context.Context, run *domain.SyncRun, err error, requestID string) (*SyncResult, error) {
	now := time.Now().UTC()
	run.Status = "failed"
	run.FinishedAt = &now
	run.ErrorCount = 1
	run.Summary = fmt.Sprintf("Sync failed: %v", err)
	_ = e.repos.SyncRuns.Save(ctx, run)

	_ = e.audit.Record(ctx, audit.RecordParams{
		Operation:        "SYNC_ZONE",
		ResourceType:     "zone",
		ResourceID:       run.ZoneID,
		RequestID:        requestID,
		Status:           "FAILED",
		ErrorCode:        domain.ErrCodeProviderError,
		ProviderResponse: err.Error(),
	})

	return nil, err
}

func computeDiffs(
	zone *domain.Zone,
	remoteSettings provider.RoutingSettings,
	localRules []domain.RoutingRule,
	remoteRules []domain.RoutingRule,
	localCatchAll *domain.CatchAllRule,
	remoteCatchAll domain.CatchAllRule,
) []domain.DiffItem {
	var diffs []domain.DiffItem

	// 1. Settings Diff
	if zone.EmailRoutingEnabled == remoteSettings.Enabled {
		diffs = append(diffs, domain.DiffItem{
			Status:       domain.DiffStatusMatched,
			ResourceType: "routing_settings",
			Identifier:   "enabled",
			LocalState:   zone.EmailRoutingEnabled,
			RemoteState:  remoteSettings.Enabled,
		})
	} else {
		diffs = append(diffs, domain.DiffItem{
			Status:       domain.DiffStatusChanged,
			ResourceType: "routing_settings",
			Identifier:   "enabled",
			LocalState:   zone.EmailRoutingEnabled,
			RemoteState:  remoteSettings.Enabled,
			Details:      "Email routing enabled state differs",
		})
	}

	// 2. Explicit Rules Diff
	localMap := make(map[string]domain.RoutingRule)
	for _, r := range localRules {
		key := r.MatcherField + ":" + r.MatcherValue
		localMap[key] = r
	}

	remoteMap := make(map[string]domain.RoutingRule)
	for _, r := range remoteRules {
		key := r.MatcherField + ":" + r.MatcherValue
		remoteMap[key] = r
	}

	// Check local rules against remote
	for key, lr := range localMap {
		if rr, ok := remoteMap[key]; ok {
			if lr.ActionType == rr.ActionType && lr.Destination == rr.Destination && lr.Enabled == rr.Enabled {
				diffs = append(diffs, domain.DiffItem{
					Status:       domain.DiffStatusMatched,
					ResourceType: "rule",
					Identifier:   lr.MatcherValue,
					LocalState:   lr,
					RemoteState:  rr,
				})
			} else {
				diffs = append(diffs, domain.DiffItem{
					Status:       domain.DiffStatusChanged,
					ResourceType: "rule",
					Identifier:   lr.MatcherValue,
					LocalState:   lr,
					RemoteState:  rr,
					Details:      "Rule destination, action, or enabled status differs",
				})
			}
		} else {
			diffs = append(diffs, domain.DiffItem{
				Status:       domain.DiffStatusLocalOnly,
				ResourceType: "rule",
				Identifier:   lr.MatcherValue,
				LocalState:   lr,
				Details:      "Rule exists locally but not in Cloudflare",
			})
		}
	}

	// Check for remote-only rules
	for key, rr := range remoteMap {
		if _, ok := localMap[key]; !ok {
			diffs = append(diffs, domain.DiffItem{
				Status:       domain.DiffStatusRemoteOnly,
				ResourceType: "rule",
				Identifier:   rr.MatcherValue,
				RemoteState:  rr,
				Details:      "Rule exists in Cloudflare but not locally",
			})
		}
	}

	// 3. Catch-All Diff
	if localCatchAll == nil {
		if remoteCatchAll.Enabled {
			diffs = append(diffs, domain.DiffItem{
				Status:       domain.DiffStatusRemoteOnly,
				ResourceType: "catch_all",
				Identifier:   "*",
				RemoteState:  remoteCatchAll,
				Details:      "Catch-all configured remotely but not tracked locally",
			})
		} else {
			diffs = append(diffs, domain.DiffItem{
				Status:       domain.DiffStatusMatched,
				ResourceType: "catch_all",
				Identifier:   "*",
				Details:      "Catch-all disabled both locally and remotely",
			})
		}
	} else {
		if localCatchAll.Enabled == remoteCatchAll.Enabled &&
			localCatchAll.ActionType == remoteCatchAll.ActionType &&
			localCatchAll.Destination == remoteCatchAll.Destination {
			diffs = append(diffs, domain.DiffItem{
				Status:       domain.DiffStatusMatched,
				ResourceType: "catch_all",
				Identifier:   "*",
				LocalState:   localCatchAll,
				RemoteState:  remoteCatchAll,
			})
		} else {
			diffs = append(diffs, domain.DiffItem{
				Status:       domain.DiffStatusChanged,
				ResourceType: "catch_all",
				Identifier:   "*",
				LocalState:   localCatchAll,
				RemoteState:  remoteCatchAll,
				Details:      "Catch-all configuration differs",
			})
		}
	}

	return diffs
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
