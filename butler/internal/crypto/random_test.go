package crypto

import (
	"encoding/hex"
	"testing"
)

func TestGeneratePassword_ReturnsNonEmpty(t *testing.T) {
	pw, err := GeneratePassword(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pw == "" {
		t.Error("expected non-empty password")
	}
}

func TestGenerateHex(t *testing.T) {
	value, err := GenerateHex(32)
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 64 {
		t.Fatalf("GenerateHex(32) length = %d, want 64", len(value))
	}
	if _, err := hex.DecodeString(value); err != nil {
		t.Fatalf("GenerateHex(32) returned non-hex value: %v", err)
	}
	if _, err := GenerateHex(0); err == nil {
		t.Fatal("GenerateHex(0) returned nil error")
	}
}

func TestGeneratePassword_Uniqueness(t *testing.T) {
	a, err := GeneratePassword(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := GeneratePassword(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == b {
		t.Error("two generated passwords should not be identical")
	}
}

func TestGeneratePassword_DifferentLengths(t *testing.T) {
	short, err := GeneratePassword(8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	long, err := GeneratePassword(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(long) <= len(short) {
		t.Errorf("longer byte length should produce longer encoded string: short=%d long=%d", len(short), len(long))
	}
}

func TestGeneratePassword_ZeroLength(t *testing.T) {
	_, err := GeneratePassword(0)
	if err == nil {
		t.Error("expected error for zero length")
	}
}

func TestGeneratePassword_NegativeLength(t *testing.T) {
	_, err := GeneratePassword(-1)
	if err == nil {
		t.Error("expected error for negative length")
	}
}
