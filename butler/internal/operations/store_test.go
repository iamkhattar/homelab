package operations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOperationLifecycleDoesNotRecordSensitiveInput(t *testing.T) {
	store := NewStore(10)
	op := store.Start(context.Background(), "identity.user.disable", "admin@example.test", func(context.Context) error {
		return errors.New("provider unavailable")
	})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		operations := store.Operations()
		if len(operations) == 1 && operations[0].State == Failed {
			if operations[0].ID != op.ID || operations[0].Error != "provider unavailable" {
				t.Fatalf("unexpected operation: %#v", operations[0])
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("operation did not finish")
}

func TestStoreIsBounded(t *testing.T) {
	store := NewStore(2)
	store.Record("one", "actor", "one")
	store.Record("two", "actor", "two")
	store.Record("three", "actor", "three")
	if got := len(store.Events()); got != 2 {
		t.Fatalf("events = %d, want 2", got)
	}
}
