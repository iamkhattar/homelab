package operations

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPersistentStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.json")
	first, err := NewPersistentStore(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	first.Record("identity.user.created", "admin@example.test", "user created")
	operation := first.Start(context.Background(), "reconcile", "admin@example.test", func(context.Context) error { return nil })
	for i := 0; i < 100 && first.Operations()[0].State != Succeeded; i++ {
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
