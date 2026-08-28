package operations

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.json")
	first, err := NewPersistentStore(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	first.Record("identity.user.created", "admin@example.test", "user created")
	operation := first.Start(context.Background(), "reconcile", "admin@example.test", func(context.Context) error { return nil })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && first.Operations()[0].State != Succeeded {
		time.Sleep(time.Millisecond)
	}
	if first.Operations()[0].State != Succeeded {
		t.Fatal("operation did not finish before persistence reload")
	}
	second, err := NewPersistentStore(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Events()) < 2 || len(second.Operations()) != 1 {
		t.Fatalf("persisted state = %#v %#v", second.Events(), second.Operations())
	}
	if second.Operations()[0].ID != operation.ID {
		t.Fatalf("operation was not restored")
	}
}
