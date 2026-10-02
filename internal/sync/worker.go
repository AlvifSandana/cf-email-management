package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bariskode/email-management-service/internal/domain"
	"github.com/bariskode/email-management-service/internal/storage"
)

// DriftCallback is invoked when drift is detected for a zone.
type DriftCallback func(zone domain.Zone, result *SyncResult)

// Worker periodically checks all managed zones against the provider to detect drift.
type Worker struct {
	repos       *storage.Repositories
	engine      *Engine
	interval    time.Duration
	logger      *slog.Logger
	onDriftFunc func(zone domain.Zone, result *SyncResult)
}

// NewWorker constructs a new scheduled background reconciliation worker.
func NewWorker(
	repos *storage.Repositories,
	engine *Engine,
	interval time.Duration,
	logger *slog.Logger,
	onDriftFunc func(zone domain.Zone, result *SyncResult),
) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Worker{
		repos:       repos,
		engine:      engine,
		interval:    interval,
		logger:      logger,
		onDriftFunc: onDriftFunc,
	}
}

func (w *Worker) getLogger() *slog.Logger {
	if w.logger != nil {
		return w.logger
	}
	return slog.Default()
}

func (w *Worker) getInterval() time.Duration {
	if w.interval > 0 {
		return w.interval
	}
	return 5 * time.Minute
}

// Repos returns the repositories configured for the worker.
func (w *Worker) Repos() *storage.Repositories {
	return w.repos
}

// Engine returns the sync engine configured for the worker.
func (w *Worker) Engine() *Engine {
	return w.engine
}

// Interval returns the configured reconciliation interval.
func (w *Worker) Interval() time.Duration {
	return w.getInterval()
}

// Logger returns the logger configured for the worker.
func (w *Worker) Logger() *slog.Logger {
	return w.getLogger()
}

// Start runs a background loop using time.NewTicker.
// It executes an initial immediate reconciliation tick, then runs periodically on each ticker tick.
// It supports graceful termination when ctx.Done() is triggered.
func (w *Worker) Start(ctx context.Context) {
	logger := w.getLogger()
	interval := w.getInterval()

	logger.Info("starting scheduled background reconciliation worker", slog.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial immediate tick
	if err := w.Reconcile(ctx); err != nil && ctx.Err() != nil {
		logger.Info("scheduled background reconciliation worker stopped during initial pass", slog.Any("error", ctx.Err()))
		return
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("scheduled background reconciliation worker stopped", slog.Any("error", ctx.Err()))
			return
		case <-ticker.C:
			if err := w.Reconcile(ctx); err != nil && ctx.Err() != nil {
				logger.Info("scheduled background reconciliation worker stopped during ticker pass", slog.Any("error", ctx.Err()))
				return
			}
		}
	}
}

// Run is an alias for Start to support callers expecting Run(ctx).
func (w *Worker) Run(ctx context.Context) {
	w.Start(ctx)
}

// Reconcile executes a single pass of drift detection across all managed zones.
func (w *Worker) Reconcile(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	logger := w.getLogger()

	if w.repos == nil || w.repos.Zones == nil || w.engine == nil {
		err := fmt.Errorf("worker missing required repositories or engine")
		logger.Error("reconciliation aborted", slog.Any("error", err))
		return err
	}

	zones, err := w.repos.Zones.List(ctx)
	if err != nil {
		logger.Error("failed to list zones for reconciliation", slog.Any("error", err))
		return err
	}

	logger.Debug("running scheduled drift check pass", slog.Int("zone_count", len(zones)))

	for _, z := range zones {
		select {
		case <-ctx.Done():
			logger.Info("reconciliation interrupted by context cancellation")
			return ctx.Err()
		default:
		}

		reqID := generateID("wkr")
		result, err := w.engine.SyncZone(ctx, z.ID, "drift_check", reqID)
		if err != nil {
			logger.Error("drift check failed for zone",
				slog.String("zone_id", z.ID),
				slog.String("zone_name", z.Name),
				slog.Any("error", err),
			)
			continue
		}

		hasDrift := false
		for _, diff := range result.Diffs {
			if diff.Status != domain.DiffStatusMatched {
				hasDrift = true
				break
			}
		}

		if hasDrift && w.onDriftFunc != nil {
			logger.Warn("drift detected for zone",
				slog.String("zone_id", z.ID),
				slog.String("zone_name", z.Name),
				slog.Int("diff_count", len(result.Diffs)),
			)
			w.onDriftFunc(z, result)
		}
	}

	return nil
}
