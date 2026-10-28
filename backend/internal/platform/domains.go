package platform

import (
	"fmt"
	"net"
	"strings"
)

func NormalizeHostname(value string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if !ValidDomain(host) || net.ParseIP(host) != nil || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") {
		return "", fmt.Errorf("enter a public hostname such as api.customer.com, without https:// or a path")
	}
	return host, nil
}

func EnsureDomains(i *Instance) {
	domains := []Domain{{Hostname: i.Hostname, Kind: "default", Verified: true, Stage: "dns_pending", CertificateStatus: "pending", UpdatedAt: Now()}}
	for _, domain := range i.Domains {
		if domain.Hostname == i.Hostname {
			domain.Kind = "default"
			domain.Verified = true
			domains[0] = domain
		} else {
			domains = append(domains, domain)
		}
	}
	i.Domains = domains
}

func NewCustomDomain(host string) Domain {
	return Domain{Hostname: host, Kind: "custom", VerificationToken: "pbm-" + RandomString(32), Stage: "verification_pending", CertificateStatus: "pending", UpdatedAt: Now()}
}
func DomainMessage(d Domain) string {
	switch d.Stage {
	case "verification_pending":
		return "Waiting for domain ownership verification"
	case "dns_pending":
		return "Waiting for DNS to point to this server"
	case "proxy_configuring":
		return "Configuring Caddy reverse proxy"
	case "certificate_pending":
		return "HTTPS certificate in progress"
	case "retrying":
		return "Certificate request retry scheduled"
	case "ready":
		return "Domain and HTTPS are ready"
	case "failed":
		return "Domain configuration needs attention"
	default:
		return "Preparing domain"
	}
}
func UpdateReadiness(i *Instance) {
	EnsureDomains(i)
	d := i.Domains[0]
	i.DNSReady = d.DNSReady
	if i.Status == "provisioning" {
		return
	}
	i.ProvisioningStage = d.Stage
	i.ProvisioningMessage = DomainMessage(d)
}
