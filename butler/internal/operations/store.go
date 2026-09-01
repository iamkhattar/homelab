// Package operations owns Butler's bounded, audit-safe operation and event log.
package operations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type State string

const (
	Pending   State = "pending"
	Running   State = "running"
	Succeeded State = "succeeded"
	Failed    State = "failed"
)

const safeOperationFailure = "operation failed; inspect Butler logs for details"

type Operation struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Actor       string    `json:"actor"`
	State       State     `json:"state"`
	CreatedAt   time.Time `json:"createdAt"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type Event struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation,omitempty"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store keeps only control-plane metadata. Request bodies, credentials and
// provider responses must never be placed in it.
type Store struct {
	mu         sync.RWMutex
	operations []Operation
	events     []Event
	limit      int
	backend    backend
}

type backend interface {
	SaveOperation(context.Context, Operation) error
	SaveEvent(context.Context, Event) error
}

func NewStore(limit int) *Store {
	if limit < 1 {
		limit = 200
	}
	return &Store{limit: limit}
}

func (s *Store) Start(ctx context.Context, kind, actor string, fn func(context.Context) error) Operation {
	op := Operation{ID: id(), Kind: kind, Actor: actor, State: Pending, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	s.operations = prependBounded(s.operations, op, s.limit)
	event := Event{ID: id(), Operation: op.ID, Type: "operation.queued", Actor: actor, Message: kind + " queued", CreatedAt: time.Now().UTC()}
	s.events = prependBounded(s.events, event, s.limit)
	s.mu.Unlock()
	s.saveOperationOrLog(context.WithoutCancel(ctx), op)
	s.saveEventOrLog(context.WithoutCancel(ctx), event)

	go func() {
		s.setState(op.ID, Running, nil)
		err := fn(context.WithoutCancel(ctx))
		if err != nil {
			s.setState(op.ID, Failed, err)
			return
		}
		s.setState(op.ID, Succeeded, nil)
	}()
	return op
}

func (s *Store) Record(eventType, actor, message string) {
	s.mu.Lock()
	event := Event{ID: id(), Type: eventType, Actor: actor, Message: message, CreatedAt: time.Now().UTC()}
	s.events = prependBounded(s.events, event, s.limit)
	s.mu.Unlock()
	s.saveEventOrLog(context.Background(), event)
}

func (s *Store) Operations() []Operation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Operation(nil), s.operations...)
}

func (s *Store) Events() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Event(nil), s.events...)
}

func (s *Store) setState(operationID string, state State, operationErr error) {
	s.mu.Lock()
	for i := range s.operations {
		if s.operations[i].ID != operationID {
			continue
		}
		s.operations[i].State = state
		if state == Succeeded || state == Failed {
			s.operations[i].CompletedAt = time.Now().UTC()
		}
		if operationErr != nil {
			s.operations[i].Error = safeOperationFailure
		}
		kind := "operation." + string(state)
		message := s.operations[i].Kind + " " + string(state)
		event := Event{ID: id(), Operation: operationID, Type: kind, Actor: s.operations[i].Actor, Message: message, CreatedAt: time.Now().UTC()}
		s.events = prependBounded(s.events, event, s.limit)
		operation := s.operations[i]
		s.mu.Unlock()
		if operationErr != nil {
			slog.Error("Butler operation failed", "operation_id", operation.ID, "kind", operation.Kind)
		}
		s.saveOperationOrLog(context.Background(), operation)
		s.saveEventOrLog(context.Background(), event)
		return
	}
	s.mu.Unlock()
}

func (s *Store) saveOperationOrLog(ctx context.Context, operation Operation) {
	if s.backend != nil {
		if err := s.backend.SaveOperation(ctx, operation); err != nil {
			slog.Error("persisting Butler operation to Kubernetes", "error", err)
		}
	}
}

func (s *Store) saveEventOrLog(ctx context.Context, event Event) {
	if s.backend != nil {
		if err := s.backend.SaveEvent(ctx, event); err != nil {
			slog.Error("persisting Butler event to Kubernetes", "error", err)
		}
	}
}

func prependBounded[T any](items []T, item T, limit int) []T {
	items = append([]T{item}, items...)
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func id() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
