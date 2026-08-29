package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamkhattar/homelab/homelabctl/internal/command"
)

func TestTrustExportReadsAuthenticatedConfigMapAndWritesValidatedBundle(t *testing.T) {
	repository := testRepository(t)
	caPath := filepath.Join(t.TempDir(), "source-ca.pem")
	ca := testCertificatePEM(t, true)
	if err := os.WriteFile(caPath, ca, 0o600); err != nil {
		t.Fatal(err)
	}
	toolDirectory := t.TempDir()
	kubectl := filepath.Join(toolDirectory, "kubectl")
	if err := os.WriteFile(kubectl, []byte("#!/bin/sh\n/bin/cat \"$HOMELAB_TEST_CA\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", toolDirectory)
	t.Setenv("HOMELAB_TEST_CA", caPath)

	output := filepath.Join(t.TempDir(), "homelab-ca.pem")
	var stdout, stderr bytes.Buffer
	runner := command.NewRunner(strings.NewReader(""), &stdout, &stderr)
	root := New(BuildInfo{}, runner)
	root.SetArgs([]string{"--repo-root", repository, "--context", "titan", "trust", "export", "--output", output})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "kubectl --context titan --namespace kube-system get configmap homelab-ca-bundle") {
		t.Fatalf("command log = %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Certificate 1 SHA-256") {
		t.Fatalf("output omitted certificate fingerprint: %q", stdout.String())
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, fingerprints, err := validateCABundle(written); err != nil || len(fingerprints) != 1 {
		t.Fatalf("written bundle validation = %v, fingerprints = %v", err, fingerprints)
	}

	second := New(BuildInfo{}, command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}))
	second.SetArgs([]string{"--repo-root", repository, "trust", "export", "--output", output})
	if err := second.Execute(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second export error = %v, want overwrite refusal", err)
	}
}

func TestValidateCABundleAcceptsOnlyCACertificates(t *testing.T) {
	ca := testCertificatePEM(t, true)
	normalized, fingerprints, err := validateCABundle(append(ca, ca...))
	if err != nil {
		t.Fatal(err)
	}
	if len(fingerprints) != 2 || len(normalized) == 0 {
		t.Fatalf("fingerprints = %v, normalized bytes = %d", fingerprints, len(normalized))
	}
	for _, invalid := range [][]byte{
		[]byte("not a certificate"),
		testCertificatePEM(t, false),
		append(append([]byte(nil), ca...), []byte("trailing data")...),
	} {
		if _, _, err := validateCABundle(invalid); err == nil {
			t.Fatal("invalid CA bundle accepted")
		}
	}
}

func TestWriteNewPublicFileRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "homelab-ca.pem")
	if err := writeNewPublicFile(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPublicFile(path, []byte("second")); err == nil {
		t.Fatal("existing CA bundle was overwritten")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "first" {
		t.Fatalf("file contents = %q", raw)
	}
}

func testCertificatePEM(t *testing.T, isCA bool) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Homelab test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  isCA,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
}
