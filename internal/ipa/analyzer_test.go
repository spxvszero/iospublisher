package ipa

import (
	"archive/zip"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iospublisher/internal/config"
)

func TestAnalyzeDevelopmentProfile(t *testing.T) {
	certExpiresAt := time.Date(2027, 6, 8, 10, 0, 0, 0, time.UTC)
	certData := testCertificate(t, certExpiresAt)
	profileExpiresAt := time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC)
	ipaPath := testIPA(t, profileXML(profileOptions{
		GetTaskAllow:       true,
		ProvisionedDevices: []string{"ABCDEF12-3456-7890-ABCD-EF1234567890", "12345678-1234-1234-1234-1234567890AB"},
		Certificates:       [][]byte{certData},
		ExpirationDate:     profileExpiresAt,
	}))

	analysis := Analyze(ipaPath)
	if analysis.Status != config.AnalysisSuccess {
		t.Fatalf("Status = %q, error = %s", analysis.Status, analysis.Error)
	}
	if analysis.PackageType != config.PackageDevelopment {
		t.Fatalf("PackageType = %q", analysis.PackageType)
	}
	if len(analysis.DeviceUUIDs) != 2 {
		t.Fatalf("DeviceUUIDs = %#v", analysis.DeviceUUIDs)
	}
	if !analysis.CertificateExpiresAt.Equal(certExpiresAt) {
		t.Fatalf("CertificateExpiresAt = %s", analysis.CertificateExpiresAt)
	}
	if !analysis.ProfileExpiresAt.Equal(profileExpiresAt) {
		t.Fatalf("ProfileExpiresAt = %s", analysis.ProfileExpiresAt)
	}
}

func TestAnalyzeEnterpriseProfile(t *testing.T) {
	ipaPath := testIPA(t, profileXML(profileOptions{ProvisionsAllDevices: true}))
	analysis := Analyze(ipaPath)
	if analysis.Status != config.AnalysisSuccess {
		t.Fatalf("Status = %q, error = %s", analysis.Status, analysis.Error)
	}
	if analysis.PackageType != config.PackageEnterprise {
		t.Fatalf("PackageType = %q", analysis.PackageType)
	}
}

func TestAnalyzeInvalidIPA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.ipa")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	analysis := Analyze(path)
	if analysis.Status != config.AnalysisFailed {
		t.Fatalf("Status = %q", analysis.Status)
	}
	if analysis.PackageType != config.PackageUnknown {
		t.Fatalf("PackageType = %q", analysis.PackageType)
	}
	if analysis.Error == "" {
		t.Fatal("Error should be recorded")
	}
}

type profileOptions struct {
	GetTaskAllow         bool
	ProvisionsAllDevices bool
	ProvisionedDevices   []string
	Certificates         [][]byte
	ExpirationDate       time.Time
}

func profileXML(opts profileOptions) string {
	if opts.ExpirationDate.IsZero() {
		opts.ExpirationDate = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<plist version="1.0"><dict>`)
	b.WriteString(`<key>Entitlements</key><dict><key>get-task-allow</key>`)
	if opts.GetTaskAllow {
		b.WriteString(`<true/>`)
	} else {
		b.WriteString(`<false/>`)
	}
	b.WriteString(`</dict>`)
	if opts.ProvisionsAllDevices {
		b.WriteString(`<key>ProvisionsAllDevices</key><true/>`)
	}
	if len(opts.ProvisionedDevices) > 0 {
		b.WriteString(`<key>ProvisionedDevices</key><array>`)
		for _, uuid := range opts.ProvisionedDevices {
			b.WriteString(`<string>` + uuid + `</string>`)
		}
		b.WriteString(`</array>`)
	}
	if len(opts.Certificates) > 0 {
		b.WriteString(`<key>DeveloperCertificates</key><array>`)
		for _, cert := range opts.Certificates {
			b.WriteString(`<data>` + base64.StdEncoding.EncodeToString(cert) + `</data>`)
		}
		b.WriteString(`</array>`)
	}
	b.WriteString(`<key>ExpirationDate</key><date>` + opts.ExpirationDate.Format(time.RFC3339) + `</date>`)
	b.WriteString(`</dict></plist>`)
	return b.String()
}

func testIPA(t *testing.T, profile string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "demo.ipa")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writer := zip.NewWriter(file)
	part, err := writer.Create("Payload/Demo.app/embedded.mobileprovision")
	if err != nil {
		t.Fatalf("zip Create() error = %v", err)
	}
	if _, err := part.Write([]byte("signed-prefix" + profile + "signed-suffix")); err != nil {
		t.Fatalf("zip Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
	return path
}

func testCertificate(t *testing.T, notAfter time.Time) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "iOS Publisher Test"},
		NotBefore:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	return der
}
