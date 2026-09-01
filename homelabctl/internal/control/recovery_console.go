package control

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

const recoveryConsoleCookie = "homelab_recovery_console"

// RecoveryConsole is a loopback-only reverse proxy that keeps the recovery
// bearer token out of browser state. A random one-time bootstrap path grants
// an HTTP-only, SameSite=Strict cookie for the lifetime of the local process.
type RecoveryConsole struct {
	URL      string
	server   *http.Server
	listener net.Listener
	done     chan error
}

func StartRecoveryConsole(upstreamAddress, token string) (*RecoveryConsole, error) {
	upstream, err := url.Parse(upstreamAddress)
	if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" {
		return nil, fmt.Errorf("parsing recovery upstream address")
	}
	hostname := upstream.Hostname()
	ip := net.ParseIP(hostname)
	if hostname != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("recovery console upstream must be loopback-only")
	}
	if token == "" {
		return nil, fmt.Errorf("recovery token is required")
	}
	capability, err := randomVaultNonce(32)
	if err != nil {
		return nil, fmt.Errorf("generating recovery console capability: %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("opening recovery console: %w", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "recovery service unavailable", http.StatusBadGateway)
	}
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		originalDirector(request)
		request.Header.Del("Cookie")
		request.Header.Set("Authorization", "Bearer "+token)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		response.Header.Set("Cache-Control", "no-store")
		response.Header.Set("X-Content-Type-Options", "nosniff")
		response.Header.Set("Referrer-Policy", "no-referrer")
		return nil
	}

	bootstrapPath := "/session/" + capability
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if request.URL.Path == bootstrapPath {
			http.SetCookie(w, &http.Cookie{
				Name:     recoveryConsoleCookie,
				Value:    capability,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
				MaxAge:   600,
			})
			http.Redirect(w, request, "/", http.StatusFound)
			return
		}
		cookie, err := request.Cookie(recoveryConsoleCookie)
		if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(capability)) != 1 {
			http.NotFound(w, request)
			return
		}
		proxy.ServeHTTP(w, request)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	return &RecoveryConsole{
		URL:      "http://" + listener.Addr().String() + bootstrapPath,
		server:   server,
		listener: listener,
		done:     done,
	}, nil
}

func (c *RecoveryConsole) Wait(ctx context.Context) error {
	select {
	case err := <-c.done:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("serving recovery console: %w", err)
		}
		return nil
	case <-ctx.Done():
		return nil
	}
}

func (c *RecoveryConsole) Close() error {
	if c == nil || c.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.server.Shutdown(ctx)
}
