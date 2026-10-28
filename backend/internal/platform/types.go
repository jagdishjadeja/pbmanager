package platform

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$`)
var idPattern = regexp.MustCompile(`^[a-z0-9]{15}$`)

func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("use 3–32 lowercase letters, numbers, or hyphens")
	}
	for _, reserved := range []string{"www", "api", "admin", "manager", "pbmanager", "mail", "localhost"} {
		if name == reserved {
			return fmt.Errorf("this instance name is reserved")
		}
	}
	return nil
}

func ValidateID(id string) bool { return idPattern.MatchString(id) }
func RandomName() string        { return RandomString(6) }
func RandomString(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for index := range result {
		var b [1]byte
		for {
			if _, err := rand.Read(b[:]); err != nil {
				panic(err)
			}
			if b[0] < 252 {
				break
			}
		}
		result[index] = alphabet[int(b[0])%len(alphabet)]
	}
	return string(result)
}

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func ValidDomain(domain string) bool {
	if len(domain) > 253 || strings.ContainsAny(domain, "/:* \t\r\n") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || !regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`).MatchString(label) {
			return false
		}
	}
	return strings.Contains(domain, ".")
}

type Instance struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Hostname            string      `json:"hostname"`
	DNSReady            bool        `json:"dnsReady"`
	Status              string      `json:"status"`
	Version             string      `json:"version"`
	Port                int         `json:"port"`
	DatabaseBytes       int64       `json:"databaseBytes"`
	StorageBytes        int64       `json:"storageBytes"`
	MemoryBytes         int64       `json:"memoryBytes"`
	MemoryLimitBytes    int64       `json:"memoryLimitBytes"`
	CPUPercent          float64     `json:"cpuPercent"`
	Requests            int64       `json:"requests"`
	IngressBytes        int64       `json:"ingressBytes"`
	IngressEstimated    bool        `json:"ingressEstimated"`
	EgressBytes         int64       `json:"egressBytes"`
	ErrorRequests       int64       `json:"errorRequests"`
	LatencyMS           float64     `json:"latencyMs"`
	QuotaBytes          int64       `json:"quotaBytes"`
	RestartCount        int         `json:"restartCount"`
	CreatedAt           string      `json:"createdAt"`
	UpdatedAt           string      `json:"updatedAt"`
	LastError           string      `json:"lastError,omitempty"`
	Domains             []Domain    `json:"domains"`
	ProvisioningStage   string      `json:"provisioningStage"`
	ProvisioningMessage string      `json:"provisioningMessage"`
	StaticSite          *StaticSite `json:"staticSite,omitempty"`
	SFTPUsers           []SFTPUser  `json:"sftpUsers,omitempty"`
}

type SFTPUser struct {
	Username  string `json:"username"`
	UID       int    `json:"uid"`
	CreatedAt string `json:"createdAt"`
}

type StaticSite struct {
	Filename    string `json:"filename"`
	Files       int    `json:"files"`
	Bytes       int64  `json:"bytes"`
	PublishedAt string `json:"publishedAt"`
}

type Storage struct {
	TotalBytes     int64  `json:"totalBytes"`
	UsedBytes      int64  `json:"usedBytes"`
	AvailableBytes int64  `json:"availableBytes"`
	InstanceBytes  int64  `json:"instanceBytes"`
	OtherBytes     int64  `json:"otherBytes"`
	MeasuredAt     string `json:"measuredAt"`
}

type Server struct {
	Name              string `json:"name"`
	OS                string `json:"os"`
	CPUCount          int    `json:"cpuCount"`
	MemoryTotalBytes  int64  `json:"memoryTotalBytes"`
	BaseDomain        string `json:"baseDomain"`
	ManagerHost       string `json:"managerHost"`
	PocketBaseVersion string `json:"pocketbaseVersion"`
	TrafficAvailable  bool   `json:"trafficAvailable"`
	PublicIP          string `json:"publicIP"`
	Error             string `json:"error,omitempty"`
}

type Activity struct {
	ID           string `json:"id"`
	InstanceID   string `json:"instanceId,omitempty"`
	InstanceName string `json:"instanceName"`
	Message      string `json:"message"`
	Kind         string `json:"kind"`
	At           string `json:"at"`
}

type Snapshot struct {
	Instances []Instance `json:"instances"`
	Activity  []Activity `json:"activity"`
	Storage   Storage    `json:"storage"`
	Server    Server     `json:"server"`
}

type Sample struct {
	At            string  `json:"at"`
	Requests      int64   `json:"requests"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryBytes   int64   `json:"memoryBytes"`
	IngressBytes  int64   `json:"ingressBytes"`
	EgressBytes   int64   `json:"egressBytes"`
	ErrorRequests int64   `json:"errorRequests"`
	LatencyMS     float64 `json:"latencyMs"`
}

type Operation struct {
	ID         string `json:"id"`
	InstanceID string `json:"instanceId"`
	Action     string `json:"action"`
	State      string `json:"state"`
	Step       int    `json:"step"`
	Label      string `json:"label"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type AgentRequest struct {
	Instance Instance `json:"instance"`
	Action   string   `json:"action"`
	Email    string   `json:"email,omitempty"`
	Password string   `json:"password,omitempty"`
	Username string   `json:"username,omitempty"`
}

type DatabaseRequest struct {
	InstanceID string          `json:"instanceId"`
	Method     string          `json:"method"`
	Path       string          `json:"path"`
	Body       json.RawMessage `json:"body,omitempty"`
}

type DatabaseResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

type AgentSnapshot struct {
	Instances []Instance `json:"instances"`
	Storage   Storage    `json:"storage"`
	Server    Server     `json:"server"`
}

type Domain struct {
	Hostname             string `json:"hostname"`
	Kind                 string `json:"kind"`
	VerificationToken    string `json:"verificationToken,omitempty"`
	Verified             bool   `json:"verified"`
	DNSReady             bool   `json:"dnsReady"`
	RouteConfigured      bool   `json:"routeConfigured"`
	Stage                string `json:"stage"`
	CertificateStatus    string `json:"certificateStatus"`
	CertificateIssuer    string `json:"certificateIssuer,omitempty"`
	CertificateExpiresAt string `json:"certificateExpiresAt,omitempty"`
	LastError            string `json:"lastError,omitempty"`
	NextRetryAt          string `json:"nextRetryAt,omitempty"`
	UpdatedAt            string `json:"updatedAt"`
}

type Progress struct {
	InstanceID string `json:"instanceId"`
	Step       int    `json:"step"`
	Label      string `json:"label"`
	UpdatedAt  string `json:"updatedAt"`
}
