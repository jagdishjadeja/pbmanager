package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"pbmanager/backend/internal/platform"
)

func TestDomainOwnershipCertificateAndPersistence(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "registry"), 0700)
	verified := false
	issued := false
	var routed []string
	s := &server{config: config{State: root}, resolveHost: func(string) (bool, error) { return true, nil }, verifyProof: func(string, string) bool { return verified }, probeCertificate: func(string) (string, string, error) {
		if !issued {
			return "", "", errors.New("certificate pending")
		}
		return "Trusted CA", time.Now().Add(90 * 24 * time.Hour).UTC().Format(time.RFC3339), nil
	}, applyRoutes: func(_ platform.Instance, hosts []string) error { routed = append([]string(nil), hosts...); return nil }}
	m := &manifest{Instance: platform.Instance{ID: "abcdefghijklmno", Hostname: "default.example.com", Status: "running"}}
	platform.EnsureDomains(&m.Instance)
	m.Instance.Domains = append(m.Instance.Domains, platform.NewCustomDomain("api.customer.example"))
	s.reconcileDomains(m, true)
	if slices.Contains(routed, "api.customer.example") {
		t.Fatal("an unverified customer hostname was routed")
	}
	if m.Instance.Domains[1].Stage != "verification_pending" {
		t.Fatal(m.Instance.Domains[1].Stage)
	}
	verified = true
	s.reconcileDomains(m, true)
	if !slices.Contains(routed, m.Instance.Hostname) || !slices.Contains(routed, "api.customer.example") {
		t.Fatal("default domain was replaced by customer domain")
	}
	if m.Instance.Domains[1].Stage != "certificate_pending" || m.Instance.Domains[1].CertificateStatus != "issuing" {
		t.Fatal("certificate was marked ready before a valid TLS handshake")
	}
	issued = true
	s.reconcileDomains(m, true)
	if err := s.save(m); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.load(m.Instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range loaded.Instance.Domains {
		if domain.Stage != "ready" || domain.CertificateStatus != "active" || domain.CertificateExpiresAt == "" {
			t.Fatalf("certificate state was not persisted: %+v", domain)
		}
	}
}

func TestLegacyInstanceKeepsItsDefaultRoute(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "registry"), 0700)
	s := &server{config: config{State: root, CaddyDir: root}}
	i := platform.Instance{ID: "abcdefghijklmno", Hostname: "existing.example.com"}
	if err := s.save(&manifest{Instance: i}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.routeFile(i.ID), []byte(i.Hostname+" { reverse_proxy 127.0.0.1:21000 }"), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := s.load(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Instance.Domains) != 1 || m.Instance.Domains[0].Kind != "default" || !m.Instance.Domains[0].RouteConfigured {
		t.Fatal("legacy default domain was not migrated")
	}
}

func TestCertificateFailuresRemainVisibleUntilRecovery(t *testing.T) {
	s := &server{resolveHost: func(string) (bool, error) { return true, nil }, probeCertificate: func(string) (string, string, error) { return "", "", errors.New("pending") }, tlsEvents: map[string]tlsEvent{"app.example.com": {Error: "ACME challenge failed", NextRetry: platform.Now()}}}
	m := &manifest{Instance: platform.Instance{Hostname: "app.example.com", Status: "running"}, ConfiguredHosts: []string{"app.example.com"}}
	platform.EnsureDomains(&m.Instance)
	m.Instance.Domains[0].RouteConfigured = true
	s.reconcileDomains(m, true)
	d := m.Instance.Domains[0]
	if d.Stage != "retrying" || d.LastError == "" || d.NextRetryAt == "" {
		t.Fatal("certificate failure was hidden")
	}
}
