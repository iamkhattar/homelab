package control

import (
	"bytes"
	"testing"
)

func TestTunnelOutputDetectsReadinessAcrossWrites(t *testing.T) {
	var destination bytes.Buffer
	ready := make(chan struct{})
	output := newTunnelOutput(&destination, ready)

	for _, fragment := range []string{"Forwarding ", "from 127.0.0.1:54321 -> 8081\n"} {
		if _, err := output.Write([]byte(fragment)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	select {
	case <-ready:
	default:
		t.Fatal("readiness was not detected across writes")
	}
	if got := destination.String(); got != "Forwarding from 127.0.0.1:54321 -> 8081\n" {
		t.Fatalf("forwarded output = %q", got)
	}
}

func TestTunnelOutputIgnoresUnrelatedMessages(t *testing.T) {
	var destination bytes.Buffer
	ready := make(chan struct{})
	output := newTunnelOutput(&destination, ready)

	if _, err := output.Write([]byte("Handling connection for 54321\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	select {
	case <-ready:
		t.Fatal("unrelated output signalled readiness")
	default:
	}
}
