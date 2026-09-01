package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Tunnel struct {
	URL  string
	cmd  *exec.Cmd
	done chan error
}

type tunnelOutput struct {
	mu          sync.Mutex
	destination io.Writer
	ready       chan struct{}
	once        sync.Once
	tail        string
}

func newTunnelOutput(destination io.Writer, ready chan struct{}) *tunnelOutput {
	return &tunnelOutput{destination: destination, ready: ready}
}

func (w *tunnelOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	written, err := w.destination.Write(data)
	w.tail += string(data)
	if strings.Contains(w.tail, "Forwarding from") {
		w.once.Do(func() { close(w.ready) })
	}
	if len(w.tail) > 512 {
		w.tail = w.tail[len(w.tail)-512:]
	}
	return written, err
}

func StartTunnel(ctx context.Context, stderr io.Writer, kubeContext, namespace, service string, remotePort int) (*Tunnel, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("allocating local port: %w", err)
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	// #nosec G204 -- every argument is separately validated by Cobra or fixed
	// by this package; no shell is involved.
	cmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext, "--namespace", namespace,
		"port-forward", "service/"+service, strconv.Itoa(localPort)+":"+strconv.Itoa(remotePort), "--address", "127.0.0.1")
	ready := make(chan struct{})
	output := newTunnelOutput(stderr, ready)
	// kubectl versions have emitted the readiness line on both stdout and
	// stderr. Observe both streams so a working forward cannot time out merely
	// because the client changed its output destination.
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting kubectl port-forward: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ready:
	case err := <-done:
		return nil, fmt.Errorf("kubectl port-forward exited before becoming ready: %w", err)
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("kubectl port-forward did not become ready within 15s")
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, ctx.Err()
	}
	return &Tunnel{URL: fmt.Sprintf("http://127.0.0.1:%d", localPort), cmd: cmd, done: done}, nil
}

func (t *Tunnel) Close() error {
	if t == nil || t.cmd == nil || t.cmd.Process == nil {
		return nil
	}
	if err := t.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case <-t.done:
	case <-time.After(2 * time.Second):
	}
	return nil
}
