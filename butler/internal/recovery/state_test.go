package recovery

import (
	"context"
	"strings"
	"testing"
)

func TestAdvanceRequiresExplicitConfirmation(t *testing.T) {
	service := &Service{}
	if err := service.Advance(context.Background(), false); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("error = %v", err)
	}
}

func TestPocketIDImportRejectsEmptyCredential(t *testing.T) {
	service := &Service{}
	if err := service.ImportPocketIDAPIKey(context.Background(), " "); err == nil {
		t.Fatal("expected empty key rejection")
	}
}
