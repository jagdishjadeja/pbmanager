package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"pbmanager/backend/internal/platform"
)

var publicResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, "1.1.1.1:53")
}}

func lookupDomain(host string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		addresses, err = publicResolver.LookupHost(ctx, host)
	}
	return len(addresses) > 0 && err == nil, err
}
func lookupProof(host, token string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, resolver := range []*net.Resolver{net.DefaultResolver, publicResolver} {
		records, err := resolver.LookupTXT(ctx, "_pbmanager."+host)
		if err == nil && slices.Contains(records, token) {
			return true
		}
	}
	return false
}

func certificate(host string) (issuer, expires string, err error) {
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", "127.0.0.1:443", &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return "", "", err
	}
	defer connection.Close()
	certificates := connection.ConnectionState().PeerCertificates
	if len(certificates) == 0 {
		return "", "", errors.New("certificate is not available")
	}
	leaf := certificates[0]
	return leaf.Issuer.CommonName, leaf.NotAfter.UTC().Format(time.RFC3339), nil
}

func domainStage(d *platform.Domain) {
	if !d.Verified {
		d.Stage = "verification_pending"
		d.CertificateStatus = "pending"
	} else if !d.DNSReady {
		d.Stage = "dns_pending"
	} else if !d.RouteConfigured {
		d.Stage = "proxy_configuring"
	} else if d.CertificateStatus == "active" {
		d.Stage = "ready"
	} else if d.CertificateStatus == "retrying" {
		d.Stage = "retrying"
	} else {
		d.Stage = "certificate_pending"
		d.CertificateStatus = "issuing"
	}
}

func (s *server) domainProgress(id string, step int, label string) {
	data, _ := json.Marshal(platform.Progress{InstanceID: id, Step: step, Label: label, UpdatedAt: platform.Now()})
	_ = atomicWrite(filepath.Join(s.config.State, "progress", id+".json"), data, 0600)
}
func (s *server) progress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !platform.ValidateID(id) {
		http.Error(w, "invalid instance", 400)
		return
	}
	data, err := os.ReadFile(filepath.Join(s.config.State, "progress", id+".json"))
	if err != nil {
		http.Error(w, "progress not available", 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (s *server) reconcileDomains(m *manifest, force bool) {
	resolve := s.resolveHost
	if resolve == nil {
		resolve = lookupDomain
	}
	proof := s.verifyProof
	if proof == nil {
		proof = lookupProof
	}
	probe := s.probeCertificate
	if probe == nil {
		probe = certificate
	}
	apply := s.applyRoutes
	if apply == nil {
		apply = s.configureRoutes
	}
	i := &m.Instance
	platform.EnsureDomains(i)
	checkDNS := force || time.Since(m.DNSChecked) > 30*time.Second
	for index := range i.Domains {
		d := &i.Domains[index]
		if checkDNS {
			if d.Kind == "custom" && !d.Verified {
				d.Verified = proof(d.Hostname, d.VerificationToken)
			}
			d.DNSReady, _ = resolve(d.Hostname)
		}
		domainStage(d)
	}
	if checkDNS {
		m.DNSChecked = time.Now()
	}
	wanted := []string{}
	for _, d := range i.Domains {
		if d.RouteConfigured || (d.Verified && d.DNSReady) {
			wanted = append(wanted, d.Hostname)
		}
	}
	slices.Sort(wanted)
	if !slices.Equal(wanted, m.ConfiguredHosts) {
		if err := apply(*i, wanted); err != nil {
			for index := range i.Domains {
				d := &i.Domains[index]
				if d.Verified && d.DNSReady && !d.RouteConfigured {
					d.Stage = "failed"
					d.LastError = err.Error()
					d.UpdatedAt = platform.Now()
				}
			}
			return
		}
		m.ConfiguredHosts = wanted
		for index := range i.Domains {
			d := &i.Domains[index]
			d.RouteConfigured = slices.Contains(wanted, d.Hostname)
			if d.RouteConfigured && d.CertificateStatus != "active" {
				d.CertificateStatus = "issuing"
				d.LastError = ""
			}
			d.UpdatedAt = platform.Now()
		}
	}
	for index := range i.Domains {
		d := &i.Domains[index]
		if !d.Verified || !d.RouteConfigured {
			continue
		}
		issuer, expires, err := probe(d.Hostname)
		if err == nil {
			d.CertificateStatus = "active"
			d.CertificateIssuer = issuer
			d.CertificateExpiresAt = expires
			d.LastError = ""
			d.NextRetryAt = ""
		} else {
			d.CertificateStatus = "issuing"
			if event, ok := s.tlsEvents[d.Hostname]; ok {
				d.CertificateStatus = "retrying"
				d.LastError = event.Error
				d.NextRetryAt = event.NextRetry
			}
		}
		domainStage(d)
		d.UpdatedAt = platform.Now()
	}
	platform.UpdateReadiness(i)
}

type tlsEvent struct{ Error, NextRetry string }

func (s *server) readTLSEvents() {
	output, err := command("journalctl", "--unit=caddy", "--since=-2h", "--lines=2000", "--no-pager", "--output=cat")
	if err != nil {
		return
	}
	s.tlsEvents = map[string]tlsEvent{}
	for _, line := range strings.Split(output, "\n") {
		var event struct {
			Identifier string  `json:"identifier"`
			Error      string  `json:"error"`
			Message    string  `json:"msg"`
			Timestamp  float64 `json:"ts"`
			Retry      float64 `json:"retrying_in"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Identifier == "" && strings.HasPrefix(event.Error, "[") {
			if end := strings.Index(event.Error, "]"); end > 1 {
				event.Identifier = event.Error[1:end]
			}
		}
		if event.Identifier == "" {
			continue
		}
		if event.Message == "certificate obtained successfully" {
			delete(s.tlsEvents, event.Identifier)
		} else if event.Error != "" {
			item := tlsEvent{Error: event.Error}
			if event.Retry > 0 {
				item.NextRetry = time.Unix(int64(event.Timestamp+event.Retry), 0).UTC().Format(time.RFC3339)
			}
			s.tlsEvents[event.Identifier] = item
		}
	}
}

func (s *server) updateDomains(m *manifest, requested []platform.Domain) error {
	if len(requested) > 6 {
		return errors.New("an instance supports up to five custom domains")
	}
	platform.EnsureDomains(&m.Instance)
	old := map[string]platform.Domain{}
	for _, d := range m.Instance.Domains {
		old[d.Hostname] = d
	}
	result := []platform.Domain{old[m.Instance.Hostname]}
	seen := map[string]bool{m.Instance.Hostname: true}
	all, err := s.all()
	if err != nil {
		return err
	}
	for _, candidate := range requested {
		if candidate.Hostname == m.Instance.Hostname {
			continue
		}
		host, err := platform.NormalizeHostname(candidate.Hostname)
		if err != nil {
			return err
		}
		if seen[host] || host == s.config.ManagerHost || host == s.config.BaseDomain || strings.HasSuffix(host, "."+s.config.BaseDomain) {
			return errors.New("this hostname is reserved or already attached")
		}
		seen[host] = true
		for _, other := range all {
			if other.Instance.ID == m.Instance.ID {
				continue
			}
			platform.EnsureDomains(&other.Instance)
			for _, domain := range other.Instance.Domains {
				if domain.Hostname == host {
					return errors.New("domain is already assigned to another instance")
				}
			}
		}
		if previous, ok := old[host]; ok {
			result = append(result, previous)
			continue
		}
		if len(candidate.VerificationToken) != 36 || !strings.HasPrefix(candidate.VerificationToken, "pbm-") {
			return errors.New("invalid domain verification token")
		}
		inUse, err := s.caddyHostnameInUse(host)
		if err != nil {
			return err
		}
		if inUse {
			return errors.New("hostname is already served by another Caddy site")
		}
		result = append(result, platform.Domain{Hostname: host, Kind: "custom", VerificationToken: candidate.VerificationToken, Stage: "verification_pending", CertificateStatus: "pending", UpdatedAt: platform.Now()})
	}
	m.Instance.Domains = result
	s.reconcileDomains(m, true)
	return s.save(m)
}

func (s *server) caddyHostnameInUse(host string) (bool, error) {
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get("http://127.0.0.1:2019/config/apps/http/servers/")
	if err != nil {
		return false, fmt.Errorf("cannot check Caddy hostname reservations: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return false, errors.New("cannot read Caddy hostname reservations")
	}
	var value any
	if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&value); err != nil {
		return false, err
	}
	var contains func(any) bool
	contains = func(value any) bool {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				if key == "host" {
					if names, ok := item.([]any); ok {
						for _, name := range names {
							if name == host {
								return true
							}
						}
					}
				}
				if contains(item) {
					return true
				}
			}
		case []any:
			for _, item := range v {
				if contains(item) {
					return true
				}
			}
		}
		return false
	}
	return contains(value), nil
}
