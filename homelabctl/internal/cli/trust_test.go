package cli

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
