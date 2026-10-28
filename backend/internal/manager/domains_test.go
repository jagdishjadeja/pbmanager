package manager

import (
	"encoding/json"
	"testing"

	"pbmanager/backend/internal/platform"
)

func TestCustomerDomainsKeepDefaultAndRejectConflicts(t *testing.T) {
	m, handler, cookie := setup(t)
	response := perform(handler, "POST", "/api/manager/instances", `{"name":"domains-demo","customDomain":"API.Customer.Example"}`, cookie)
	if response.Code != 202 {
		t.Fatal(response.Body.String())
	}
	var created struct {
		Instance platform.Instance `json:"instance"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &created)
	if len(created.Instance.Domains) != 2 || created.Instance.Domains[0].Kind != "default" || created.Instance.Domains[1].Hostname != "api.customer.example" {
		t.Fatal("default and customer domain were not reserved")
	}
	m.work()
	defaultURL := "/api/manager/instances/" + created.Instance.ID + "/domains/" + created.Instance.Hostname
	if result := perform(handler, "DELETE", defaultURL, "", cookie); result.Code != 400 {
		t.Fatal("default domain could be removed")
	}
	duplicate := perform(handler, "POST", "/api/manager/instances", `{"name":"second-demo","customDomain":"api.customer.example"}`, cookie)
	if duplicate.Code != 400 {
		t.Fatal("customer domain was allocated twice")
	}
	for _, hostname := range []string{"https://bad.example/path", "*.customer.example", "manager.example.com", "stolen.pb.example.com", "127.0.0.1"} {
		body, _ := json.Marshal(map[string]string{"hostname": hostname})
		result := perform(handler, "POST", "/api/manager/instances/"+created.Instance.ID+"/domains", string(body), cookie)
		if result.Code != 400 {
			t.Fatalf("invalid or reserved domain accepted: %s %d", hostname, result.Code)
		}
	}
	domainURL := "/api/manager/instances/" + created.Instance.ID + "/domains/api.customer.example"
	if result := perform(handler, "DELETE", domainURL, "", cookie); result.Code != 202 {
		t.Fatal(result.Body.String())
	}
	m.work()
	i, err := m.find(created.Instance.ID)
	if err != nil || len(i.Domains) != 1 || i.Domains[0].Hostname != i.Hostname {
		t.Fatal("removing a customer domain changed the default")
	}
}

func TestCertificateReadinessIsWrittenToPocketBase(t *testing.T) {
	m, handler, cookie := setup(t)
	response := perform(handler, "POST", "/api/manager/instances", `{"name":"cert-demo"}`, cookie)
	var created struct {
		Instance platform.Instance `json:"instance"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &created)
	m.work()
	fake := m.agent.(*fakeAgent)
	i := fake.items[created.Instance.ID]
	platform.EnsureDomains(&i)
	i.Domains[0].DNSReady = true
	i.Domains[0].RouteConfigured = true
	i.Domains[0].CertificateStatus = "active"
	i.Domains[0].CertificateIssuer = "Trusted CA"
	i.Domains[0].CertificateExpiresAt = "2027-01-01T00:00:00Z"
	i.Domains[0].Stage = "ready"
	fake.items[i.ID] = i
	m.refresh()
	record, err := m.app.FindRecordById("manager_instances", i.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := decode[platform.Instance](record)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ProvisioningStage != "ready" || stored.Domains[0].CertificateStatus != "active" || stored.Domains[0].CertificateExpiresAt == "" {
		t.Fatal("certificate completion did not update PocketBase")
	}
}
