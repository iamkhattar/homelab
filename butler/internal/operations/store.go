// Package operations owns Butler's bounded, audit-safe operation and event log.
package operations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	path       string
}

type snapshot struct {
	Operations []Operation `json:"operations"`
	Events     []Event     `json:"events"`
}

func NewStore(limit int) *Store {
	if limit < 1 {
		limit = 200
	}
	return &Store{limit: limit}
}

func NewPersistentStore(path string, limit int) (*Store, error) {
	store := NewStore(limit)
	store.path = path
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating operations directory: %w", err)
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- path is fixed deployment configuration.
	if err != nil {
		if os.IsNotExist(err) {
			if err := store.persistLocked(); err != nil {
				return nil, err
			}
			return store, nil
		}
		return nil, fmt.Errorf("reading operations state: %w", err)
	}
	var state snapshot
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("decoding operations state: %w", err)
	}
	store.operations = state.Operations
	store.events = state.Events
	if len(store.operations) > store.limit {
		store.operations = store.operations[:store.limit]
	}
	if len(store.events) > store.limit {
		store.events = store.events[:store.limit]
	}
	return store, nil
}

func (s *Store) Start(ctx context.Context, kind, actor string, fn func(context.Context) error) Operation {
	op := Operation{ID: id(), Kind: kind, Actor: actor, State: Pending, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	s.operations = prependBounded(s.operations, op, s.limit)
	s.events = prependBounded(s.events, Event{ID: id(), Operation: op.ID, Type: "operation.queued", Actor: actor, Message: kind + " queued", CreatedAt: time.Now().UTC()}, s.limit)
	s.persistOrLogLocked()
	s.mu.Unlock()

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
	defer s.mu.Unlock()
	s.events = prependBounded(s.events, Event{ID: id(), Type: eventType, Actor: actor, Message: message, CreatedAt: time.Now().UTC()}, s.limit)
	s.persistOrLogLocked()
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
	defer s.mu.Unlock()
	for i := range s.operations {
		if s.operations[i].ID != operationID {
			continue
		}
		s.operations[i].State = state
		if state == Succeeded || state == Failed {
			s.operations[i].CompletedAt = time.Now().UTC()
		}
		if operationErr != nil {
			s.operations[i].Error = operationErr.Error()
		}
		kind := "operation." + string(state)
		message := s.operations[i].Kind + " " + string(state)
		s.events = prependBounded(s.events, Event{ID: id(), Operation: operationID, Type: kind, Actor: s.operations[i].Actor, Message: message, CreatedAt: time.Now().UTC()}, s.limit)
		s.persistOrLogLocked()
		return
	}
}

func (s *Store) persistOrLogLocked() {
	if err := s.persistLocked(); err != nil {
		slog.Error("persisting Butler operations", "error", err)
	}
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	raw, err := json.Marshal(snapshot{Operations: s.operations, Events: s.events})
	if err != nil {
		return fmt.Errorf("encoding operations state: %w", err)
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return fmt.Errorf("writing operations state: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("securing operations state: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("installing operations state: %w", err)
	}
	return nil
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
