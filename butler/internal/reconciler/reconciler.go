package reconciler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

// Reconciler is implemented by any component that reconciles desired state.
type Reconciler interface {
	Name() string
	Reconcile(ctx context.Context) error
}

// Status holds the result of the last reconciliation for a single reconciler.
type Status struct {
	Name     string    `json:"name"`
	LastRun  time.Time `json:"last_run"`
	Success  bool      `json:"success"`
	Error    string    `json:"error,omitempty"`
	Duration string    `json:"duration"`
}

// Scheduler serializes initial, event-triggered, API-triggered, and periodic
// reconciliation passes.
type Scheduler struct {
	reconcilers []Reconciler
	interval    time.Duration

	runMu    sync.Mutex
	mu       sync.RWMutex
	statuses map[string]Status
}

var (
	reconcileRuns, _     = otel.Meter("butler/reconciler").Int64Counter("butler.reconcile.runs")
	reconcileDuration, _ = otel.Meter("butler/reconciler").Float64Histogram("butler.reconcile.duration", metric.WithUnit("s"))
)

// NewScheduler creates a scheduler that ticks at the given interval.
func NewScheduler(interval time.Duration, reconcilers ...Reconciler) *Scheduler {
	return &Scheduler{
		reconcilers: reconcilers,
		interval:    interval,
		statuses:    make(map[string]Status),
	}
}

// RunOnce executes all reconcilers sequentially and returns the first error.
func (s *Scheduler) RunOnce(ctx context.Context) error {
	// Provider creates and one-time secret responses are not safe to reconcile
	// concurrently. Serialize timer- and API-triggered runs in this replica.
	s.runMu.Lock()
	defer s.runMu.Unlock()

	var failures []error
	for _, r := range s.reconcilers {
		start := time.Now()
		reconcileCtx, span := otel.Tracer("butler/reconciler").Start(ctx, "reconcile "+r.Name())
		span.SetAttributes(attribute.String("butler.reconciler.name", r.Name()))
		err := r.Reconcile(reconcileCtx)
		dur := time.Since(start)
		attrs := metric.WithAttributes(attribute.String("butler.reconciler.name", r.Name()), attribute.Bool("butler.reconciler.success", err == nil))
		reconcileRuns.Add(reconcileCtx, 1, attrs)
		reconcileDuration.Record(reconcileCtx, dur.Seconds(), attrs)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "reconciliation failed")
		}
		span.End()

		status := Status{
			Name:     r.Name(),
			LastRun:  start,
			Success:  err == nil,
			Duration: dur.String(),
		}
		if err != nil {
			// Provider errors can contain response bodies or sensitive metadata.
			// The authenticated status API exposes only a stable safe message.
			status.Error = "reconciliation failed; inspect Butler logs for details"
			slog.ErrorContext(reconcileCtx, "reconciler failed", "reconciler", r.Name(), "error", err, "duration", dur, "trace_id", span.SpanContext().TraceID().String())
		} else {
			slog.InfoContext(reconcileCtx, "reconciler succeeded", "reconciler", r.Name(), "duration", dur, "trace_id", span.SpanContext().TraceID().String())
		}

		s.mu.Lock()
		s.statuses[r.Name()] = status
		s.mu.Unlock()

		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// Start runs the reconciliation loop until the context is cancelled. An
// optional coalescing trigger allows Kubernetes desired-state events to run an
// immediate pass while the interval remains a periodic drift-repair resync.
func (s *Scheduler) Start(ctx context.Context, triggers ...<-chan struct{}) {
	slog.Info("starting reconciliation loop", "interval", s.interval)
	var trigger <-chan struct{}
	if len(triggers) > 0 {
		trigger = triggers[0]
	}

	if err := s.RunOnce(ctx); err != nil {
		slog.Error("initial reconciliation failed", "error", err)
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("reconciliation loop stopped")
			return
		case _, ok := <-trigger:
			if !ok {
				trigger = nil
				continue
			}
			if err := s.RunOnce(ctx); err != nil {
				slog.Error("event-triggered reconciliation failed", "error", err)
			}
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				slog.Error("reconciliation failed", "error", err)
			}
		}
	}
}

// Statuses returns a snapshot of all reconciler statuses.
func (s *Scheduler) Statuses() []Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Status, 0, len(s.statuses))
	for _, st := range s.statuses {
		out = append(out, st)
	}
	return out
}
