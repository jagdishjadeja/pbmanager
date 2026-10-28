package manager

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/agent"
	"pbmanager/backend/internal/platform"
)

type Agent interface {
	Execute(platform.AgentRequest) error
	Snapshot() (platform.AgentSnapshot, error)
	Logs(string) ([]string, error)
	PublishSite(string, string, string, io.Reader) (platform.StaticSite, error)
	RemoveSite(string) error
}
type Config struct {
	AgentSocket, BaseDomain, ManagerHost string
	Key                                  []byte
	SecureCookies                        bool
}
type Manager struct {
	app           core.App
	agent         Agent
	config        Config
	mutation      sync.Mutex
	cache         sync.RWMutex
	snapshot      platform.Snapshot
	wake          chan struct{}
	loginMutex    sync.Mutex
	loginAttempts map[string][]time.Time
}

func FromEnvironment(app core.App) (*Manager, error) {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("PBM_ENCRYPTION_KEY"))
	if err != nil || len(key) != 32 {
		return nil, errors.New("PBM_ENCRYPTION_KEY must contain a base64-encoded 32-byte key")
	}
	config := Config{AgentSocket: os.Getenv("PBM_AGENT_SOCKET"), BaseDomain: os.Getenv("PBM_BASE_DOMAIN"), ManagerHost: os.Getenv("PBM_MANAGER_HOST"), Key: key, SecureCookies: os.Getenv("PBM_SECURE_COOKIES") != "false"}
	if config.AgentSocket == "" {
		config.AgentSocket = "/run/pbmanager-agent/agent.sock"
	}
	if !platform.ValidDomain(config.BaseDomain) || !platform.ValidDomain(config.ManagerHost) {
		return nil, errors.New("PBM_BASE_DOMAIN and PBM_MANAGER_HOST must be configured")
	}
	return New(app, agent.NewClient(config.AgentSocket), config)
}

func New(app core.App, agent Agent, config Config) (*Manager, error) {
	if len(config.Key) != 32 {
		return nil, errors.New("encryption key must contain 32 bytes")
	}
	if err := ensureCollections(app); err != nil {
		return nil, err
	}
	m := &Manager{app: app, agent: agent, config: config, wake: make(chan struct{}, 1), loginAttempts: map[string][]time.Time{}}
	m.snapshot.Instances = []platform.Instance{}
	m.snapshot.Activity = []platform.Activity{}
	m.snapshot.Server = platform.Server{BaseDomain: config.BaseDomain, ManagerHost: config.ManagerHost}
	return m, nil
}

func (m *Manager) Start(ctx context.Context) {
	// Replaying an interrupted operation is safe: the agent owns a durable registry.
	records, err := m.app.FindRecordsByFilter("manager_operations", "state='running'", "", 0, 0)
	if err == nil {
		for _, record := range records {
			operation, _ := decode[platform.Operation](record)
			operation.State = "queued"
			operation.Label = "Resuming interrupted operation"
			_ = saveOperation(m.app, operation)
		}
	}
	m.refresh()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.refresh()
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.wake:
			case <-ticker.C:
			}
			for {
				if ctx.Err() != nil {
					return
				}
				if !m.work() {
					break
				}
			}
		}
	}()
}

func (m *Manager) event(i platform.Instance, message, kind string) {
	a := platform.Activity{ID: platform.RandomString(15), InstanceID: i.ID, InstanceName: i.Name, Message: message, Kind: kind, At: platform.Now()}
	if _, err := put(m.app, "manager_activity", "", a, nil); err != nil {
		log.Printf("save activity: %v", err)
	}
}

func (m *Manager) refresh() {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	result, agentErr := m.agent.Snapshot()
	stored, err := instances(m.app)
	if err != nil {
		log.Printf("read instances: %v", err)
		return
	}
	measured := map[string]platform.Instance{}
	for _, item := range result.Instances {
		measured[item.ID] = item
	}
	for index, item := range stored {
		if current, ok := measured[item.ID]; ok {
			item.DatabaseBytes = current.DatabaseBytes
			item.DNSReady = current.DNSReady
			platform.EnsureDomains(&item)
			for index, desired := range item.Domains {
				for _, observed := range current.Domains {
					if observed.Hostname == desired.Hostname {
						item.Domains[index] = observed
						break
					}
				}
			}
			item.StorageBytes = current.StorageBytes
			item.StaticSite = current.StaticSite
			item.SFTPUsers = current.SFTPUsers
			item.CPUPercent = current.CPUPercent
			item.MemoryBytes = current.MemoryBytes
			item.MemoryLimitBytes = current.MemoryLimitBytes
			item.Requests = current.Requests
			item.IngressBytes = current.IngressBytes
			item.IngressEstimated = current.IngressEstimated
			item.EgressBytes = current.EgressBytes
			item.ErrorRequests = current.ErrorRequests
			item.LatencyMS = current.LatencyMS
			item.RestartCount = current.RestartCount
			item.QuotaBytes = current.QuotaBytes
			active, _ := m.app.FindRecordsByFilter("manager_operations", "instance={:id} && (state='queued' || state='running')", "", 1, 0, dbx.Params{"id": item.ID})
			if len(active) == 0 && !(item.Status == "failed" && item.LastError != "") {
				if item.Status != current.Status {
					item.UpdatedAt = platform.Now()
				}
				item.Status = current.Status
			}
			if current.LastError != "" {
				item.LastError = current.LastError
			}
			_ = saveInstance(m.app, item)
		} else if agentErr == nil && item.Status != "provisioning" {
			item.Status = "failed"
			item.LastError = "Instance is missing from the provisioning agent registry."
			_ = saveInstance(m.app, item)
		}
		stored[index] = item
		platform.UpdateReadiness(&item)
		if activeJobs, lookupErr := m.app.FindRecordsByFilter("manager_operations", "instance={:id} && (state='queued' || state='running')", "", 1, 0, dbx.Params{"id": item.ID}); lookupErr == nil && len(activeJobs) > 0 && item.Status == "provisioning" {
			operation, _ := decode[platform.Operation](activeJobs[0])
			item.ProvisioningStage = "provisioning"
			item.ProvisioningMessage = operation.Label
		}
		_ = saveInstance(m.app, item)
		stored[index] = item
	}
	activity := []platform.Activity{}
	records, err := m.app.FindRecordsByFilter("manager_activity", "", "-created", 100, 0)
	if err == nil {
		for _, record := range records {
			item, err := decode[platform.Activity](record)
			if err == nil {
				activity = append(activity, item)
			}
		}
	}
	m.cache.Lock()
	m.snapshot.Instances = stored
	m.snapshot.Activity = activity
	if agentErr == nil {
		m.snapshot.Storage = result.Storage
		m.snapshot.Server = result.Server
	} else {
		m.snapshot.Server.Error = "Live measurements are unavailable: " + agentErr.Error()
	}
	m.cache.Unlock()
	if agentErr == nil {
		m.recordMetrics(stored)
	}
}

func (m *Manager) recordMetrics(items []platform.Instance) {
	for _, item := range items {
		previous, err := m.app.FindRecordsByFilter("manager_metrics", "instance={:id}", "-time", 1, 0, dbx.Params{"id": item.ID})
		if err != nil {
			continue
		}
		if len(previous) > 0 && time.Now().Unix()-int64(previous[0].GetInt("time")) < 60 {
			continue
		}
		totals := platform.Sample{Requests: item.Requests, IngressBytes: item.IngressBytes, EgressBytes: item.EgressBytes, ErrorRequests: item.ErrorRequests}
		sample := platform.Sample{At: platform.Now(), CPUPercent: item.CPUPercent, MemoryBytes: item.MemoryBytes, LatencyMS: item.LatencyMS}
		if len(previous) > 0 {
			var last platform.Sample
			_ = previous[0].UnmarshalJSONField("totals", &last)
			sample.Requests = max(0, totals.Requests-last.Requests)
			sample.IngressBytes = max(0, totals.IngressBytes-last.IngressBytes)
			sample.EgressBytes = max(0, totals.EgressBytes-last.EgressBytes)
			sample.ErrorRequests = max(0, totals.ErrorRequests-last.ErrorRequests)
		}
		_, _ = put(m.app, "manager_metrics", "", sample, map[string]any{"instance": item.ID, "time": time.Now().Unix(), "totals": totals})
	}
	_, _ = m.app.DB().NewQuery("DELETE FROM manager_metrics WHERE time < {:cutoff}").Bind(dbx.Params{"cutoff": time.Now().Add(-30 * 24 * time.Hour).Unix()}).Execute()
}

func (m *Manager) enqueue(i platform.Instance, action string, payload platform.AgentRequest) (platform.Operation, error) {
	active, err := m.app.FindRecordsByFilter("manager_operations", "instance={:id} && (state='queued' || state='running')", "", 1, 0, dbx.Params{"id": i.ID})
	if err != nil {
		return platform.Operation{}, err
	}
	if len(active) > 0 {
		return platform.Operation{}, errors.New("another operation is in progress")
	}
	operation := platform.Operation{InstanceID: i.ID, Action: action, State: "queued", Step: 0, Label: "Waiting for provisioning agent", CreatedAt: platform.Now(), UpdatedAt: platform.Now()}
	secret, err := seal(m.config.Key, payload)
	if err != nil {
		return operation, err
	}
	record, err := put(m.app, "manager_operations", "", operation, map[string]any{"instance": i.ID, "state": "queued", "secret": secret})
	if err != nil {
		return operation, err
	}
	operation.ID = record.Id
	if err = saveOperation(m.app, operation); err != nil {
		return operation, err
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return operation, nil
}

func (m *Manager) work() bool {
	m.mutation.Lock()
	records, err := m.app.FindRecordsByFilter("manager_operations", "state='queued'", "created", 1, 0)
	if err != nil || len(records) == 0 {
		m.mutation.Unlock()
		return false
	}
	record := records[0]
	operation, err := decode[platform.Operation](record)
	if err != nil {
		m.mutation.Unlock()
		return false
	}
	operation.State = "running"
	operation.Label = "Applying systemd service and Caddy configuration"
	operation.Step = 1
	operation.UpdatedAt = platform.Now()
	if err = saveOperation(m.app, operation); err != nil {
		m.mutation.Unlock()
		return false
	}
	var request platform.AgentRequest
	err = unseal(m.config.Key, record.GetString("secret"), &request)
	m.mutation.Unlock()
	if err == nil {
		done := make(chan error, 1)
		go func() { done <- m.agent.Execute(request) }()
		ticker := time.NewTicker(500 * time.Millisecond)
		waiting := true
		for waiting {
			select {
			case err = <-done:
				waiting = false
			case <-ticker.C:
				if progressAgent, ok := m.agent.(interface {
					Progress(string) (platform.Progress, error)
				}); ok {
					if progress, progressErr := progressAgent.Progress(operation.InstanceID); progressErr == nil {
						operation.Step = progress.Step
						operation.Label = progress.Label
						operation.UpdatedAt = progress.UpdatedAt
						_ = saveOperation(m.app, operation)
					}
				}
			}
		}
		ticker.Stop()
	}
	m.mutation.Lock()
	operation.UpdatedAt = platform.Now()
	instanceRecord, findErr := m.app.FindRecordById("manager_instances", operation.InstanceID)
	var item platform.Instance
	if findErr == nil {
		item, _ = decode[platform.Instance](instanceRecord)
		item.ID = instanceRecord.Id
	}
	if err != nil {
		operation.State = "failed"
		operation.Error = err.Error()
		operation.Label = "Operation failed"
		if findErr == nil {
			if operation.Action != "domains" && operation.Action != "resources" && operation.Action != "sftp-create" && operation.Action != "sftp-remove" {
				item.Status = "failed"
			} else if operation.Action == "domains" {
				for index := range item.Domains {
					if item.Domains[index].Kind == "custom" {
						item.Domains[index].Stage = "failed"
						item.Domains[index].LastError = err.Error()
					}
				}
			}
			item.LastError = err.Error()
			item.UpdatedAt = platform.Now()
			_ = saveInstance(m.app, item)
			m.event(item, operation.Action+" failed: "+err.Error(), "error")
		}
	} else {
		operation.State = "succeeded"
		operation.Step = 5
		operation.Label = "Operation completed"
		if findErr == nil {
			if operation.Action == "delete" {
				_ = m.app.Delete(instanceRecord)
				oldOperations, _ := m.app.FindRecordsByFilter("manager_operations", "instance={:id}", "", 0, 0, dbx.Params{"id": item.ID})
				for _, old := range oldOperations {
					old.Set("secret", "")
					_ = m.app.Save(old)
				}
			} else {
				if operation.Action != "domains" && operation.Action != "resources" && operation.Action != "sftp-create" && operation.Action != "sftp-remove" {
					item.Status = "running"
				}
				if operation.Action == "stop" {
					item.Status = "stopped"
				}
				item.LastError = ""
				item.UpdatedAt = platform.Now()
				_ = saveInstance(m.app, item)
			}
			m.event(item, fmt.Sprintf("Instance %s completed", operation.Action), "success")
		}
		if operation.Action != "create" {
			record.Set("secret", "")
			_ = m.app.Save(record)
		}
	}
	_ = saveOperation(m.app, operation)
	m.mutation.Unlock()
	m.refresh()
	return true
}
