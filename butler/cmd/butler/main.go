package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iamkhattar/homelab/butler/internal/access"
	"github.com/iamkhattar/homelab/butler/internal/certificates"
	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/identity"
	"github.com/iamkhattar/homelab/butler/internal/observability"
	"github.com/iamkhattar/homelab/butler/internal/operations"
	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/reconciler"
	"github.com/iamkhattar/homelab/butler/internal/recovery"
	"github.com/iamkhattar/homelab/butler/internal/server"
	"github.com/iamkhattar/homelab/butler/internal/vault"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
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
	shutdownTelemetry, err := observability.Setup(ctx, version)
	if err != nil {
		slog.Error("setting up telemetry", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			slog.Error("shutting down telemetry", "error", err)
		}
	}()
	slog.Info("butler starting", "version", version, "commit", commit, "built", date)

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
	dynamicK8s, err := dynamic.NewForConfig(k8sCfg)
	if err != nil {
		slog.Error("creating dynamic k8s client", "error", err)
		os.Exit(1)
	}
	platformResources := platform.NewStore(dynamicK8s)

	// Vault client.
	vc, err := vault.NewClient(cfg.Vault.Address)
	if err != nil {
		slog.Error("creating vault client", "error", err)
		os.Exit(1)
	}

	mode := os.Getenv("BUTLER_MODE")
	if mode == "" {
		mode = "normal"
	}

	// The recovery runtime is the only process allowed to initialize or unseal
	// Vault. The normal runtime authenticates with its projected Kubernetes
	// identity and only performs idempotent configuration/reconciliation.
	vaultConfiguration := reconciler.NewVaultConfiguration(vc, k8s, cfg.Namespace, cfg)
	if mode == "recovery" {
		acmeDNSClient, err := certificates.NewClient(cfg.Certificates.ACMEDNSURL)
		if err != nil {
			slog.Error("configuring acme-dns client", "error", err)
			os.Exit(1)
		}
		certificateManager, err := certificates.NewManager(certificates.Config{
			APIURL: cfg.Certificates.ACMEDNSURL, Domain: cfg.Certificates.Domain,
			CredentialPath: cfg.Certificates.CredentialPath, CertificateNS: cfg.Certificates.Namespace,
			CertificateName: cfg.Certificates.CertificateName, TLSSecretName: cfg.Certificates.TLSSecretName,
		}, vc, acmeDNSClient, nil)
		if err != nil {
			slog.Error("configuring certificate bootstrap", "error", err)
			os.Exit(1)
		}
		credentialBootstrap := reconciler.NewManagedCredentialBootstrap(vc, platformResources, cfg.Namespace, "pocket-id-runtime")
		identityBootstrap := recovery.Sequence{Steps: []recovery.Bootstrapper{
			reconciler.NewPocketIDGroups(vc, platformResources, cfg.OIDC.AdminURL),
			reconciler.NewPocketIDClients(vc, platformResources, cfg.OIDC.AdminURL),
		}}
		serveRecovery(ctx, cfg, vc, k8s, reconciler.NewVaultBootstrap(vc, k8s, cfg.Namespace, cfg), credentialBootstrap, identityBootstrap, certificateManager)
		return
	}
	if mode != "normal" {
		slog.Error("unsupported BUTLER_MODE", "mode", mode)
		os.Exit(1)
	}
	if cfg.OIDC.Issuer == "" {
		slog.Error("normal mode requires OIDC; refusing to expose an unauthenticated control plane")
		os.Exit(1)
	}

	reconcilers := []reconciler.Reconciler{
		vaultConfiguration,
		reconciler.NewManagedCredentials(vc, platformResources),
		reconciler.NewPocketIDGroups(vc, platformResources, cfg.OIDC.AdminURL),
		reconciler.NewPocketIDClients(vc, platformResources, cfg.OIDC.AdminURL),
	}
	if cfg.Garage.Enabled {
		reconcilers = append(reconcilers, reconciler.NewGarage(vc, cfg.Garage, platformResources))
	}

	sched := reconciler.NewScheduler(cfg.ReconcileInterval, reconcilers...)
	platformChanges, err := platformResources.WatchChanges(ctx)
	if err != nil {
		slog.Error("watching platform resources", "error", err)
		os.Exit(1)
	}

	auth, err := server.NewAuthMiddleware(ctx, cfg.OIDC.Issuer, cfg.OIDC.Audience, nil)
	if err != nil {
		slog.Error("setting up auth middleware", "error", err)
		os.Exit(1)
	}
	auth.AllowAudiences(cfg.OIDC.AllowedAudiences...)

	srv := server.New(sched, vc, auth)
	operationsStore, err := operations.NewKubernetesStore(ctx, dynamicK8s, cfg.Namespace, 500)
	if err != nil {
		slog.Error("opening Kubernetes operations store", "error", err)
		os.Exit(1)
	}
	srv.UseOperationsStore(operationsStore)
	credentialService, err := access.NewCredentialService(vc, cfg.K8sIssuance)
	if err != nil {
		slog.Error("configuring Kubernetes credential issuance", "error", err)
		os.Exit(1)
	}
	srv.ConfigureControlPlane(identity.NewService(vc, cfg.OIDC.AdminURL, platformResources), credentialService)

	// Start reconciliation loop in background.
	go sched.Start(ctx, platformChanges)

	// Start HTTP server.
	httpSrv := &http.Server{
		Addr:              ":" + cfg.Server.Port,
		Handler:           otelhttp.NewHandler(server.RequestLogger(srv), "butler.http"),
		ReadHeaderTimeout: 10 * time.Second,
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

func serveRecovery(ctx context.Context, cfg *config.Config, vc *vault.Client, k8s kubernetes.Interface, bootstrapper, credentialBootstrapper, identityBootstrapper recovery.Bootstrapper, certificateManager *certificates.Manager) {
	// This succeeds after the one-time bootstrap has created the narrow
	// recovery role. It is intentionally best-effort for the pristine-cluster
	// case, where that role cannot exist yet. Re-authenticating here also makes
	// post-bootstrap API-key recovery work after a recovery pod restart.
	if err := vc.LoginKubernetes(ctx, "butler-recovery", cfg.Vault.KubernetesAuth.TokenPath); err != nil {
		slog.Info("recovery Vault role is not available yet", "error", err)
	}
	auth := recovery.NewTokenReviewer(k8s, cfg.Namespace, "butler-recovery-client")
	recoveryService := recovery.NewService(vc, k8s, cfg.Namespace, bootstrapper, credentialBootstrapper, identityBootstrapper)
	recoveryService.UseCertificates(certificateManager)
	srv := server.NewRecovery(auth, recoveryService)
	httpSrv := &http.Server{Addr: ":" + cfg.Server.Port, Handler: otelhttp.NewHandler(server.RequestLogger(srv), "butler.recovery.http"), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		slog.Info("recovery server listening", "port", cfg.Server.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("recovery server error", "error", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("recovery server shutdown", "error", err)
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
