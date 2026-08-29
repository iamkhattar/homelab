package cli

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	caBundleNamespace = "kube-system"
	caBundleConfigMap = "homelab-ca-bundle"
)

func newTrustCommand(s *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "trust",
		Short: "Establish workstation trust from the authenticated cluster",
		Long:  "Export public trust material through the existing Kubernetes administrator connection. This avoids trusting a certificate merely because an unauthenticated HTTPS endpoint served it.",
	}
	var output string
	export := &cobra.Command{
		Use:     "export",
		Short:   "Export and validate the homelab PKI CA bundle",
		Long:    "Read the public homelab CA chain from Butler's well-known kube-system ConfigMap, validate every PEM certificate, write a new file without overwriting an existing path, and print the SHA-256 fingerprint of each certificate.",
		Example: "  homelabctl trust export --output /secure/homelab-ca.pem",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			absolute, err := filepath.Abs(output)
			if err != nil {
				return fmt.Errorf("resolving CA bundle output: %w", err)
			}
			if _, err := validateExternalDestination(s.root, filepath.Dir(absolute), "CA bundle destination"); err != nil {
				return err
			}
			args := []string{"--context", s.kubeContext, "--namespace", caBundleNamespace, "get", "configmap", caBundleConfigMap, "--output=jsonpath={.data.ca-bundle\\.pem}"}
			if s.dryRun {
				return s.run(cmd.Context(), s.root, "kubectl", args...)
			}
			raw, err := s.output(cmd.Context(), s.root, "kubectl", args...)
			if err != nil {
				return fmt.Errorf("reading authenticated CA bundle: %w", err)
			}
			bundle, fingerprints, err := validateCABundle([]byte(raw))
			if err != nil {
				return err
			}
			if err := writeNewPublicFile(absolute, bundle); err != nil {
				return err
			}
			s.success("validated CA bundle written to " + absolute)
			for index, fingerprint := range fingerprints {
				s.keyValue(fmt.Sprintf("Certificate %d SHA-256", index+1), fingerprint)
			}
			return nil
		},
	}
	export.Flags().StringVar(&output, "output", "", "new CA bundle path outside the repository")
	_ = export.MarkFlagRequired("output")
	command.AddCommand(export)
	return command
}

func validateCABundle(raw []byte) ([]byte, []string, error) {
	rest := raw
	var normalized []byte
	var fingerprints []string
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, nil, fmt.Errorf("CA bundle contains an unsupported PEM block")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing CA certificate: %w", err)
		}
		if !certificate.IsCA {
			return nil, nil, fmt.Errorf("CA bundle contains a certificate without CA capability")
		}
		digest := sha256.Sum256(certificate.Raw)
		fingerprints = append(fingerprints, strings.ToUpper(hex.EncodeToString(digest[:])))
		normalized = append(normalized, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})...)
		rest = remaining
	}
	if len(fingerprints) == 0 {
		return nil, nil, fmt.Errorf("CA bundle contained no certificates")
	}
	if strings.TrimSpace(string(rest)) != "" {
		return nil, nil, fmt.Errorf("CA bundle contains non-PEM data")
	}
	return normalized, fingerprints, nil
}

func writeNewPublicFile(path string, contents []byte) error {
	// The path is explicitly selected by the operator and the contents are a
	// public CA certificate intended to be readable by local TLS clients.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302,G304
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("CA bundle output already exists: %s", path)
		}
		return fmt.Errorf("creating CA bundle output: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(contents); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("writing CA bundle output: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("syncing CA bundle output: %w", err)
	}
	return nil
}
