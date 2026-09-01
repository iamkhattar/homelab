package reconciler

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// mockReconciler is a test double for the Reconciler interface.
type mockReconciler struct {
	name   string
	err    error
	called int
}

func (m *mockReconciler) Name() string { return m.name }
func (m *mockReconciler) Reconcile(_ context.Context) error {
	m.called++
	return m.err
}

func TestScheduler_RunOnce_AllSucceed(t *testing.T) {
	r1 := &mockReconciler{name: "a"}
	r2 := &mockReconciler{name: "b"}
	s := NewScheduler(time.Minute, r1, r2)

	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r1.called != 1 {
		t.Errorf("r1 called %d times, want 1", r1.called)
	}
	if r2.called != 1 {
		t.Errorf("r2 called %d times, want 1", r2.called)
	}
}

func TestScheduler_RunOnce_ContinuesAfterError(t *testing.T) {
	r1 := &mockReconciler{name: "a", err: fmt.Errorf("boom")}
	r2 := &mockReconciler{name: "b"}
	s := NewScheduler(time.Minute, r1, r2)

	err := s.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if r1.called != 1 {
		t.Errorf("r1 called %d times, want 1", r1.called)
	}
	if r2.called != 1 {
		t.Errorf("r2 called %d times, want 1", r2.called)
	}
}

func TestScheduler_Statuses(t *testing.T) {
	r1 := &mockReconciler{name: "a"}
	s := NewScheduler(time.Minute, r1)

	// Before any run, statuses should be empty.
	if len(s.Statuses()) != 0 {
		t.Errorf("expected 0 statuses before run, got %d", len(s.Statuses()))
	}

	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	statuses := s.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Name != "a" {
		t.Errorf("expected name 'a', got %q", statuses[0].Name)
	}
	if !statuses[0].Success {
		t.Error("expected success=true")
	}
	if statuses[0].Error != "" {
		t.Errorf("expected no error, got %q", statuses[0].Error)
	}
}

func TestScheduler_Statuses_RecordsError(t *testing.T) {
	r1 := &mockReconciler{name: "fail", err: fmt.Errorf("broken")}
	s := NewScheduler(time.Minute, r1)

	_ = s.RunOnce(context.Background())

	statuses := s.Statuses()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Success {
		t.Error("expected success=false")
	}
	if statuses[0].Error != "reconciliation failed; inspect Butler logs for details" {
		t.Errorf("unexpected safe error: %q", statuses[0].Error)
	}
}

func TestSchedulerSerializesConcurrentRuns(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	r := ReconcilerFunc{"serialized", func(context.Context) error {
		entered <- struct{}{}
		<-release
		return nil
	}}
	s := NewScheduler(time.Minute, r)
	done := make(chan struct{}, 2)
	go func() { _ = s.RunOnce(context.Background()); done <- struct{}{} }()
	<-entered
	go func() { _ = s.RunOnce(context.Background()); done <- struct{}{} }()
	select {
	case <-entered:
		t.Fatal("second reconciliation entered before the first completed")
	case <-time.After(25 * time.Millisecond):
	}
	release <- struct{}{}
	<-entered
	release <- struct{}{}
	<-done
	<-done
}

type ReconcilerFunc struct {
	name string
	fn   func(context.Context) error
}

func (r ReconcilerFunc) Name() string                        { return r.name }
func (r ReconcilerFunc) Reconcile(ctx context.Context) error { return r.fn(ctx) }

func TestScheduler_Start_CancelStops(t *testing.T) {
	r1 := &mockReconciler{name: "a"}
	s := NewScheduler(50*time.Millisecond, r1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	// Let at least one tick happen.
	time.Sleep(120 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("Start did not return after context cancellation")
	}

	if r1.called < 2 {
		t.Errorf("expected at least 2 calls (initial + tick), got %d", r1.called)
	}
}

func TestNewScheduler_Empty(t *testing.T) {
	s := NewScheduler(time.Minute)
	if err := s.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce with no reconcilers should succeed, got: %v", err)
	}
	if len(s.Statuses()) != 0 {
		t.Error("expected 0 statuses with no reconcilers")
	}
}
