package manager

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
)

func (m *Manager) customDomain(host string, owner string) (platform.Domain, error) {
	host, err := platform.NormalizeHostname(host)
	if err != nil {
		return platform.Domain{}, err
	}
	if host == m.config.ManagerHost || host == m.config.BaseDomain || strings.HasSuffix(host, "."+m.config.BaseDomain) {
		return platform.Domain{}, errors.New("platform hostnames are reserved; enter a customer-owned domain")
	}
	items, err := instances(m.app)
	if err != nil {
		return platform.Domain{}, err
	}
	for _, item := range items {
		platform.EnsureDomains(&item)
		for _, d := range item.Domains {
			if d.Hostname == host {
				return platform.Domain{}, errors.New("this domain is already assigned to an instance")
			}
		}
	}
	return platform.NewCustomDomain(host), nil
}

func (m *Manager) addDomain(e *core.RequestEvent) error    { return m.changeDomain(e, "add") }
func (m *Manager) removeDomain(e *core.RequestEvent) error { return m.changeDomain(e, "remove") }
func (m *Manager) verifyDomain(e *core.RequestEvent) error { return m.changeDomain(e, "verify") }
func (m *Manager) changeDomain(e *core.RequestEvent, action string) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.find(e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("Instance not found.", nil)
	}
	platform.EnsureDomains(&i)
	previous := i
	previous.Domains = append([]platform.Domain(nil), i.Domains...)
	if action == "add" {
		if len(i.Domains) >= 6 {
			return e.BadRequestError("An instance supports up to five custom domains.", nil)
		}
		var body struct {
			Hostname string `json:"hostname"`
		}
		e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, 4096)
		if err = json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
			return e.BadRequestError("Enter a domain hostname.", nil)
		}
		domain, err := m.customDomain(body.Hostname, i.ID)
		if err != nil {
			return e.BadRequestError(err.Error(), nil)
		}
		i.Domains = append(i.Domains, domain)
	} else {
		host := e.Request.PathValue("hostname")
		found := false
		domains := []platform.Domain{}
		for _, domain := range i.Domains {
			if domain.Hostname == host {
				found = true
				if action == "remove" {
					if domain.Kind == "default" {
						return e.BadRequestError("The default domain is permanent.", nil)
					}
					continue
				}
			}
			domains = append(domains, domain)
		}
		if !found {
			return e.NotFoundError("Domain not found.", nil)
		}
		i.Domains = domains
	}
	if err = saveInstance(m.app, i); err != nil {
		return e.InternalServerError("Could not save domains.", err)
	}
	operation, err := m.enqueue(i, "domains", platform.AgentRequest{Instance: i, Action: "domains"})
	if err != nil {
		_ = saveInstance(m.app, previous)
		return e.Error(409, err.Error(), nil)
	}
	return e.JSON(202, operation)
}
