package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iamkhattar/homelab/services/butler/internal/config"
	"github.com/iamkhattar/homelab/services/butler/internal/reconciler"
	"github.com/iamkhattar/homelab/services/butler/internal/server"
	"github.com/iamkhattar/homelab/services/butler/internal/vault"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	initLogging(cfg.LogLevel)
	slog.Info("butler starting")

	// Kubernetes client (in-cluster).
	k8sCfg, err := rest.InClusterConfig()
	if err != nil {
		slog.Error("building k8s config", "error", err)
		os.Exit(1)
	}
	k8s, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		slog.Error("creating k8s client", "error", err)
		os.Exit(1)
	}

	// Vault client.
	vc, err := vault.NewClient(cfg.Vault.Address)
	if err != nil {
		slog.Error("creating vault client", "error", err)
		os.Exit(1)
	}

	// Build reconcilers — order matters:
	//   1. vault-bootstrap: init + unseal + enable all engines/auth methods.
	//   2. ca-bundle: publish PKI CA chain to a ConfigMap (depends on #1).
	//   3. secrets: materialize KV-v2 secrets (depends on #1).
	//   4. oauth-clients: provision OAuth clients in Pocket-ID, persist
	//      creds to Vault (depends on #1 + Pocket-ID being reachable).
	reconcilers := []reconciler.Reconciler{
		reconciler.NewVaultBootstrap(vc, k8s, cfg.Namespace, cfg),
		reconciler.NewCABundle(vc, k8s),
	}
	if len(cfg.Secrets) > 0 {
		reconcilers = append(reconcilers, reconciler.NewSecrets(vc, cfg.Secrets))
	}
	if len(cfg.OAuthClients) > 0 {
		// Pocket-ID's admin API lives on the same host as the OIDC issuer
		// (it's the same app). Derived from cfg.OIDC.Issuer so operators
		// only configure one URL.
		reconcilers = append(reconcilers,
			reconciler.NewOAuthClients(vc, cfg.OIDC.Issuer, cfg.OAuthClients),
		)
	}

	sched := reconciler.NewScheduler(cfg.ReconcileInterval, reconcilers...)

	// Auth middleware (nil if OIDC not configured — endpoints unprotected).
	auth, err := server.NewAuthMiddleware(ctx, cfg.OIDC.Issuer)
	if err != nil {
		slog.Error("setting up auth middleware", "error", err)
		os.Exit(1)
	}

	srv := server.New(sched, vc, auth)

	// Start reconciliation loop in background.
	go sched.Start(ctx)

	// Start HTTP server.
	httpSrv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: srv,
	}

	go func() {
		slog.Info("http server listening", "port", cfg.Server.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown", "error", err)
	}
}

func initLogging(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
