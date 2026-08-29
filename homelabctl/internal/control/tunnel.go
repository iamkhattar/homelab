package control

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Tunnel struct {
	URL  string
	cmd  *exec.Cmd
	done chan error
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
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("capturing kubectl port-forward output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting kubectl port-forward: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ready := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(io.TeeReader(pipe, stderr))
		signalled := false
		for scanner.Scan() {
			if !signalled && strings.Contains(scanner.Text(), "Forwarding from") {
				ready <- nil
				signalled = true
			}
		}
		if !signalled {
			ready <- scanner.Err()
		}
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("waiting for kubectl port-forward: %w", err)
		}
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
	if err := t.cmd.Process.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
		return err
	}
	select {
	case <-t.done:
	case <-time.After(2 * time.Second):
	}
	return nil
}
