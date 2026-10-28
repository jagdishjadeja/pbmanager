package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
)

const quotaBytes int64 = 1 << 30

type config struct{ Root, State, Binary, BaseDomain, ManagerHost, Socket, SocketGroup, CaddyDir, CaddyConfig, LogDir, Version string }
type manifest struct {
	Instance        platform.Instance `json:"instance"`
	CPU             uint64            `json:"cpu"`
	CPUAt           time.Time         `json:"cpuAt"`
	LogOffset       int64             `json:"logOffset"`
	LogInode        uint64            `json:"logInode"`
	TrafficDuration float64           `json:"trafficDuration"`
	DNSChecked      time.Time         `json:"dnsChecked"`
	ConfiguredHosts []string          `json:"configuredHosts"`
}
type server struct {
	config           config
	mutex            sync.Mutex
	tlsEvents        map[string]tlsEvent
	resolveHost      func(string) (bool, error)
	verifyProof      func(string, string) bool
	probeCertificate func(string) (string, string, error)
	applyRoutes      func(platform.Instance, []string) error
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func Run() error {
	c := config{Root: env("PBM_INSTANCE_ROOT", "/var/lib/pbmanager/instances"), State: env("PBM_AGENT_STATE", "/var/lib/pbmanager-agent"), Binary: env("PBM_POCKETBASE_BINARY", "/opt/pbmanager/bin/pocketbase"), BaseDomain: env("PBM_BASE_DOMAIN", "formics.online"), ManagerHost: env("PBM_MANAGER_HOST", "pb.formics.online"), Socket: env("PBM_AGENT_SOCKET", "/run/pbmanager-agent/agent.sock"), SocketGroup: env("PBM_SOCKET_GROUP", "pbmanager"), CaddyDir: env("PBM_CADDY_DIR", "/etc/caddy/sites.d"), CaddyConfig: env("PBM_CADDY_CONFIG", "/etc/caddy/Caddyfile"), LogDir: env("PBM_LOG_DIR", "/var/log/caddy"), Version: env("PBM_POCKETBASE_VERSION", "0.39.0")}
	if !platform.ValidDomain(c.BaseDomain) || !platform.ValidDomain(c.ManagerHost) {
		return errors.New("invalid deployment domain")
	}
	for _, dir := range []string{c.Root, c.State, filepath.Join(c.State, "registry"), filepath.Join(c.State, "volumes"), filepath.Join(c.State, "progress"), filepath.Dir(c.Socket)} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	if err := os.Chmod(c.Root, 0711); err != nil {
		return err
	}
	if _, err := os.Stat(c.Socket); err == nil {
		connection, dialErr := net.DialTimeout("unix", c.Socket, time.Second)
		if dialErr == nil {
			connection.Close()
			return errors.New("another agent is running")
		}
		if err = os.Remove(c.Socket); err != nil {
			return err
		}
	}
	listener, err := net.Listen("unix", c.Socket)
	if err != nil {
		return err
	}
	group, err := user.LookupGroup(c.SocketGroup)
	if err != nil {
		return err
	}
	gid, _ := strconv.Atoi(group.Gid)
	if err = os.Chown(c.Socket, 0, gid); err != nil {
		return err
	}
	if err = os.Chmod(c.Socket, 0660); err != nil {
		return err
	}
	s := &server{config: c}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /execute", s.execute)
	mux.HandleFunc("GET /snapshot", s.snapshot)
	mux.HandleFunc("GET /logs/{id}", s.logs)
	mux.HandleFunc("GET /progress/{id}", s.progress)
	mux.HandleFunc("POST /sites/{id}", s.uploadSite)
	mux.HandleFunc("DELETE /sites/{id}", s.deleteSite)
	mux.HandleFunc("POST /database", s.databaseRequest)
	return (&http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 3 * time.Minute, IdleTimeout: 30 * time.Second}).Serve(listener)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func command(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %s", filepath.Base(name), strings.TrimSpace(string(data)))
	}
	return string(data), nil
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, mode); err != nil {
		return err
	}
	// The agent's restrictive umask must not make Caddy snippets unreadable.
	if err := os.Chmod(temporary, mode); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
func (s *server) path(id string) string { return filepath.Join(s.config.Root, id) }
func (s *server) registry(id string) string {
	return filepath.Join(s.config.State, "registry", id+".json")
}
func (s *server) unit(id string) string { return "pbm-" + id + ".service" }
func (s *server) mountUnit(id string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s.path(id), "/"), "/", "-") + ".mount"
}
func (s *server) volume(id string) string { return filepath.Join(s.config.State, "volumes", id+".img") }
func (s *server) routeFile(id string) string {
	return filepath.Join(s.config.CaddyDir, "pbmanager-"+id+".caddy")
}
func (s *server) save(m *manifest) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return atomicWrite(s.registry(m.Instance.ID), data, 0600)
}
func (s *server) load(id string) (*manifest, error) {
	if !platform.ValidateID(id) {
		return nil, errors.New("invalid instance identifier")
	}
	data, err := os.ReadFile(s.registry(id))
	if err != nil {
		return nil, err
	}
	var m manifest
	err = json.Unmarshal(data, &m)
	if err == nil && m.Instance.ID != id {
		return nil, errors.New("invalid registry entry")
	}
	if err == nil && len(m.Instance.Domains) == 0 {
		platform.EnsureDomains(&m.Instance)
		if route, readErr := os.ReadFile(s.routeFile(id)); readErr == nil && strings.Contains(string(route), m.Instance.Hostname) {
			m.Instance.Domains[0].RouteConfigured = true
			m.ConfiguredHosts = []string{m.Instance.Hostname}
		}
	}
	return &m, err
}
func (s *server) all() ([]*manifest, error) {
	entries, err := os.ReadDir(filepath.Join(s.config.State, "registry"))
	if err != nil {
		return nil, err
	}
	result := []*manifest{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		m, err := s.load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, nil
}

func (s *server) execute(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request platform.AgentRequest
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid operation", 400)
		return
	}
	i := request.Instance
	if !platform.ValidateID(i.ID) || platform.ValidateName(i.Name) != nil || i.Hostname != i.Name+"."+s.config.BaseDomain || i.Hostname == s.config.ManagerHost || i.Port < 21000 || i.Port > 29999 {
		http.Error(w, "invalid instance descriptor", 400)
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var err error
	if request.Action == "create" {
		err = s.create(request)
	} else {
		m, loadErr := s.load(i.ID)
		if loadErr != nil {
			if request.Action == "delete" && errors.Is(loadErr, os.ErrNotExist) {
				writeJSON(w, map[string]bool{"ok": true})
				return
			}
			http.Error(w, "unknown managed instance", 404)
			return
		}
		if m.Instance.Name != i.Name || m.Instance.Port != i.Port || m.Instance.Hostname != i.Hostname {
			http.Error(w, "instance descriptor does not match registry", 409)
			return
		}
		switch request.Action {
		case "sftp-create":
			s.domainProgress(i.ID, 1, "Creating the scoped SSH/SFTP file account")
			err = s.createSFTP(m, request.Username, request.Password)
		case "sftp-remove":
			err = s.removeSFTP(m, request.Username)
		case "start", "stop", "restart":
			_, err = command("systemctl", request.Action, s.unit(i.ID))
			if err == nil && request.Action != "stop" {
				err = s.healthy(i)
			}
		case "delete":
			err = s.remove(m)
		case "resources":
			s.domainProgress(i.ID, 1, "Applying the RAM limit")
			err = s.updateMemory(m, i.MemoryLimitBytes)
		case "domains":
			s.domainProgress(i.ID, 1, "Checking domain ownership and DNS")
			err = s.updateDomains(m, i.Domains)
		default:
			http.Error(w, "unsupported operation", 400)
			return
		}
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) create(request platform.AgentRequest) error {
	i := request.Instance
	if i.MemoryLimitBytes == 0 {
		i.MemoryLimitBytes = platform.DefaultMemoryMiB * platform.MiB
	}
	if err := validateHostMemory(i.MemoryLimitBytes); err != nil {
		return err
	}
	s.domainProgress(i.ID, 1, "Preparing the instance storage volume")
	items, err := s.all()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Instance.ID != i.ID && (item.Instance.Name == i.Name || item.Instance.Port == i.Port) {
			return errors.New("instance name or port is already reserved")
		}
	}
	if _, err = s.load(i.ID); errors.Is(err, os.ErrNotExist) {
		if inUse, lookupErr := s.caddyHostnameInUse(i.Hostname); lookupErr != nil {
			return lookupErr
		} else if inUse {
			return errors.New("default hostname is already served by another Caddy site")
		}
		var disk syscall.Statfs_t
		if err = syscall.Statfs(s.config.Root, &disk); err != nil {
			return err
		}
		if int64(disk.Bavail)*int64(disk.Bsize) < quotaBytes+(2<<30) {
			return errors.New("insufficient disk space: keep at least 2 GiB free after allocating the instance")
		}
		initial := i
		initial.Domains = nil
		platform.EnsureDomains(&initial)
		m := &manifest{Instance: initial}
		if err = s.save(m); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	username := "pbm-" + i.ID
	if _, err = user.Lookup(username); err != nil {
		if _, err = command("useradd", "--system", "--no-create-home", "--shell", "/usr/sbin/nologin", username); err != nil {
			return err
		}
	}
	account, err := user.Lookup(username)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	if err = os.MkdirAll(s.path(i.ID), 0750); err != nil {
		return err
	}
	if _, err = os.Stat(s.volume(i.ID)); errors.Is(err, os.ErrNotExist) {
		if _, err = command("fallocate", "-l", strconv.FormatInt(quotaBytes, 10), s.volume(i.ID)); err != nil {
			return err
		}
		if _, err = command("mkfs.ext4", "-F", "-m", "0", "-E", "lazy_itable_init=0,lazy_journal_init=0", s.volume(i.ID)); err != nil {
			_ = os.Remove(s.volume(i.ID))
			return err
		}
	} else if err != nil {
		return err
	}
	mount := fmt.Sprintf("[Unit]\nDescription=PocketBase instance storage %s\nBefore=%s\n[Mount]\nWhat=%s\nWhere=%s\nType=ext4\nOptions=loop,nodev,nosuid,noexec\n[Install]\nWantedBy=multi-user.target\n", i.ID, s.unit(i.ID), s.volume(i.ID), s.path(i.ID))
	if err = atomicWrite(filepath.Join("/etc/systemd/system", s.mountUnit(i.ID)), []byte(mount), 0644); err != nil {
		return err
	}
	if _, err = command("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if _, err = command("systemctl", "enable", "--now", s.mountUnit(i.ID)); err != nil {
		return err
	}
	if err = os.Chown(s.path(i.ID), uid, gid); err != nil {
		return err
	}
	dataDir := filepath.Join(s.path(i.ID), "pb_data")
	s.domainProgress(i.ID, 2, "Initializing PocketBase and its administrator account")
	{
		_, _ = command("systemctl", "stop", s.unit(i.ID))
		if len(request.Password) < 20 || !strings.Contains(request.Email, "@") {
			return errors.New("initial administrator credentials are required")
		}
		if err = initialize(dataDir, request.Email, request.Password); err != nil {
			return err
		}
		if err = filepath.WalkDir(dataDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("unexpected symlink in new instance")
			}
			return os.Chown(path, uid, gid)
		}); err != nil {
			return err
		}
	}
	service := fmt.Sprintf("[Unit]\nDescription=Managed PocketBase %s\nAfter=network-online.target %s\nWants=network-online.target\nRequires=%s\n[Service]\nType=simple\nUser=%s\nGroup=%s\nWorkingDirectory=%s\nExecStart=%s serve --http=127.0.0.1:%d --dir=%s --hooksDir=%s --migrationsDir=%s --publicDir=%s\nRestart=on-failure\nRestartSec=3\nTimeoutStopSec=30\nLimitNOFILE=4096\nUMask=0077\nNoNewPrivileges=true\nPrivateTmp=true\nProtectHome=true\nProtectSystem=strict\nReadWritePaths=%s\nRestrictSUIDSGID=true\nCPUAccounting=true\nMemoryAccounting=true\nMemoryMax=%d\nCPUQuota=100%%\nTasksMax=128\n[Install]\nWantedBy=multi-user.target\n", i.Name, s.mountUnit(i.ID), s.mountUnit(i.ID), username, username, s.path(i.ID), s.config.Binary, i.Port, dataDir, filepath.Join(s.path(i.ID), "pb_hooks"), filepath.Join(s.path(i.ID), "pb_migrations"), filepath.Join(s.path(i.ID), "pb_public"), s.path(i.ID), i.MemoryLimitBytes)
	if err = atomicWrite(filepath.Join("/etc/systemd/system", s.unit(i.ID)), []byte(service), 0644); err != nil {
		return err
	}
	if _, err = command("systemctl", "daemon-reload"); err != nil {
		return err
	}
	s.domainProgress(i.ID, 3, "Starting the dedicated systemd service")
	if _, err = command("systemctl", "enable", "--now", s.unit(i.ID)); err != nil {
		return err
	}
	if err = s.healthy(i); err != nil {
		return err
	}
	s.domainProgress(i.ID, 4, "Configuring the default domain and Caddy reverse proxy")
	m, err := s.load(i.ID)
	if err != nil {
		return err
	}
	if err = s.updateDomains(m, i.Domains); err != nil {
		return err
	}
	s.domainProgress(i.ID, 5, "Service created; checking domain and HTTPS certificate status")
	return nil
}

func initialize(dataDir, email, password string) error {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: dataDir})
	if err := app.Bootstrap(); err != nil {
		return err
	}
	defer app.ResetBootstrapState()
	if _, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, email); err == nil {
		return nil
	}
	collection, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.SetEmail(email)
	record.SetPassword(password)
	return app.Save(record)
}

func (s *server) healthy(i platform.Instance) error {
	client := &http.Client{Timeout: 2 * time.Second}
	for attempt := 0; attempt < 25; attempt++ {
		response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", i.Port))
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
	return errors.New("PocketBase service did not become healthy; inspect its service logs")
}

func (s *server) configureRoutes(i platform.Instance, hosts []string) error {
	if len(hosts) == 0 {
		if err := os.Remove(s.routeFile(i.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return s.reloadCaddy()
	}
	logPath := filepath.Join(s.config.LogDir, "pbm-"+i.ID+".json")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	logFile.Close()
	account, err := user.Lookup("caddy")
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	if err = os.Chown(logPath, uid, gid); err != nil {
		return err
	}
	if err = os.Chmod(logPath, 0640); err != nil {
		return err
	}
	content := fmt.Sprintf("%s {\n request_body {\n  max_size 64MB\n }\n reverse_proxy 127.0.0.1:%d\n log {\n  output file %s {\n   roll_size 10MiB\n   roll_keep 3\n   roll_keep_for 72h\n   roll_uncompressed\n  }\n  format json\n }\n}\n", strings.Join(hosts, ", "), i.Port, filepath.Join(s.config.LogDir, "pbm-"+i.ID+".json"))
	path := s.routeFile(i.ID)
	old, oldErr := os.ReadFile(path)
	if err := atomicWrite(path, []byte(content), 0644); err != nil {
		return err
	}
	if err := s.reloadCaddy(); err != nil {
		if oldErr == nil {
			_ = atomicWrite(path, old, 0644)
		} else {
			_ = os.Remove(path)
		}
		return err
	}
	return nil
}
func (s *server) reloadCaddy() error {
	if _, err := command("caddy", "validate", "--config", s.config.CaddyConfig, "--adapter", "caddyfile"); err != nil {
		return err
	}
	_, err := command("systemctl", "reload", "caddy")
	return err
}

func (s *server) remove(m *manifest) error {
	id := m.Instance.ID
	accounts := append([]platform.SFTPUser(nil), m.Instance.SFTPUsers...)
	for _, account := range accounts {
		if err := s.removeSFTP(m, account.Username); err != nil {
			return err
		}
	}
	route := s.routeFile(id)
	old, err := os.ReadFile(route)
	if err == nil {
		if err = os.Remove(route); err != nil {
			return err
		}
		if err = s.reloadCaddy(); err != nil {
			_ = atomicWrite(route, old, 0644)
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, _ = command("systemctl", "disable", "--now", s.unit(id))
	if _, statErr := os.Stat(filepath.Join("/etc/systemd/system", s.mountUnit(id))); statErr == nil {
		if _, err = command("systemctl", "disable", "--now", s.mountUnit(id)); err != nil {
			return err
		}
	}
	for _, path := range []string{filepath.Join("/etc/systemd/system", s.unit(id)), filepath.Join("/etc/systemd/system", s.mountUnit(id)), s.volume(id)} {
		if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, base := range []string{"/etc/systemd/system.control", "/run/systemd/system.control"} {
		if err = os.RemoveAll(filepath.Join(base, s.unit(id)+".d")); err != nil {
			return err
		}
	}
	if err = os.RemoveAll(s.path(id)); err != nil {
		return err
	}
	if _, err = command("systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, _ = command("userdel", "pbm-"+id)
	return os.Remove(s.registry(id))
}

func (s *server) snapshot(w http.ResponseWriter, r *http.Request) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	items, err := s.all()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.readTLSEvents()
	result := platform.AgentSnapshot{Instances: []platform.Instance{}}
	for _, item := range items {
		s.reconcileDomains(item, false)
		s.measure(item)
		result.Instances = append(result.Instances, item.Instance)
		result.Storage.InstanceBytes += item.Instance.StorageBytes
		_ = s.save(item)
	}
	var stat syscall.Statfs_t
	if err = syscall.Statfs(s.config.Root, &stat); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	result.Storage.TotalBytes = int64(stat.Blocks) * int64(stat.Bsize)
	result.Storage.AvailableBytes = int64(stat.Bavail) * int64(stat.Bsize)
	result.Storage.UsedBytes = int64(stat.Blocks-stat.Bfree) * int64(stat.Bsize)
	result.Storage.OtherBytes = max(0, result.Storage.UsedBytes-result.Storage.InstanceBytes)
	result.Storage.MeasuredAt = platform.Now()
	hostname, _ := os.Hostname()
	osName := "Linux"
	data, _ := os.ReadFile("/etc/os-release")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			osName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	mem, _ := os.ReadFile("/proc/meminfo")
	var memory int64
	for _, line := range strings.Split(string(mem), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			memory, _ = strconv.ParseInt(fields[1], 10, 64)
			memory *= 1024
		}
	}
	result.Server = platform.Server{Name: hostname, OS: osName, CPUCount: runtime.NumCPU(), MemoryTotalBytes: memory, BaseDomain: s.config.BaseDomain, ManagerHost: s.config.ManagerHost, PocketBaseVersion: s.config.Version, TrafficAvailable: true, PublicIP: env("PBM_PUBLIC_IP", "46.62.175.23")}
	writeJSON(w, result)
}

func (s *server) measure(m *manifest) {
	i := &m.Instance

	output, err := command("systemctl", "show", s.unit(i.ID), "--property=ActiveState,SubState,MemoryCurrent,MemoryMax,CPUUsageNSec,NRestarts")
	fields := map[string]string{}
	if err == nil {
		for _, line := range strings.Split(output, "\n") {
			key, value, found := strings.Cut(line, "=")
			if found {
				fields[key] = value
			}
		}
	}
	i.Status = "stopped"
	if fields["ActiveState"] == "active" {
		i.Status = "running"
	} else if fields["ActiveState"] == "failed" || fields["SubState"] == "auto-restart" {
		i.Status = "failed"
	}
	i.MemoryBytes, _ = strconv.ParseInt(fields["MemoryCurrent"], 10, 64)
	i.MemoryLimitBytes, _ = strconv.ParseInt(fields["MemoryMax"], 10, 64)
	if i.MemoryLimitBytes <= 0 {
		i.MemoryLimitBytes = platform.DefaultMemoryMiB * platform.MiB
	}
	i.RestartCount, _ = strconv.Atoi(fields["NRestarts"])
	cpu, _ := strconv.ParseUint(fields["CPUUsageNSec"], 10, 64)
	now := time.Now()
	i.CPUPercent = 0
	if !m.CPUAt.IsZero() && cpu >= m.CPU && now.Sub(m.CPUAt).Seconds() > 0 {
		i.CPUPercent = float64(cpu-m.CPU) / float64(now.Sub(m.CPUAt).Nanoseconds()) * 100
	}
	m.CPU = cpu
	m.CPUAt = now
	if i.Status != "running" {
		i.MemoryBytes = 0
		i.CPUPercent = 0
	}
	i.DatabaseBytes = 0
	i.StorageBytes = 0
	i.QuotaBytes = quotaBytes
	err = filepath.WalkDir(s.path(i.ID), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			i.StorageBytes += stat.Blocks * 512
		}
		if !entry.IsDir() && strings.HasPrefix(path, filepath.Join(s.path(i.ID), "pb_data")+string(filepath.Separator)) && (strings.HasSuffix(path, ".db") || strings.HasSuffix(path, ".db-wal") || strings.HasSuffix(path, ".db-journal") || strings.HasSuffix(path, ".db-shm")) {
			i.DatabaseBytes += info.Size()
		}
		return nil
	})
	if err != nil {
		i.LastError = "Storage measurement failed: " + err.Error()
	} else {
		i.LastError = ""
	}
	s.traffic(m)
	if len(i.SFTPUsers) > 0 {
		s.measurePublicFiles(i)
	}
}

func (s *server) traffic(m *manifest) {
	file, err := os.Open(filepath.Join(s.config.LogDir, "pbm-"+m.Instance.ID+".json"))
	if err != nil {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	if m.LogInode != stat.Ino || info.Size() < m.LogOffset {
		if m.LogInode != 0 && m.LogInode != stat.Ino {
			entries, _ := os.ReadDir(s.config.LogDir)
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasPrefix(entry.Name(), "pbm-"+m.Instance.ID) {
					continue
				}
				candidate, openErr := os.Open(filepath.Join(s.config.LogDir, entry.Name()))
				if openErr != nil {
					continue
				}
				candidateInfo, statErr := candidate.Stat()
				if statErr == nil {
					if oldStat, ok := candidateInfo.Sys().(*syscall.Stat_t); ok && oldStat.Ino == m.LogInode {
						readTraffic(candidate, m)
						candidate.Close()
						break
					}
				}
				candidate.Close()
			}
		}
		m.LogOffset = 0
		m.LogInode = stat.Ino
	}
	readTraffic(file, m)
}

func readTraffic(file *os.File, m *manifest) {
	if _, err := file.Seek(m.LogOffset, io.SeekStart); err != nil {
		return
	}
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil {
			break
		}
		m.LogOffset += int64(len(line))
		var row struct {
			BytesRead *int64  `json:"bytes_read"`
			Size      int64   `json:"size"`
			Status    int     `json:"status"`
			Duration  float64 `json:"duration"`
			Request   struct {
				Headers http.Header `json:"headers"`
			} `json:"request"`
		}
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		m.Instance.Requests++
		if row.BytesRead != nil {
			m.Instance.IngressBytes += max(0, *row.BytesRead)
		} else {
			m.Instance.IngressEstimated = true
			length, _ := strconv.ParseInt(row.Request.Headers.Get("Content-Length"), 10, 64)
			if row.Status != 413 && length > 0 && length <= 64<<20 {
				m.Instance.IngressBytes += length
			}
		}
		m.Instance.EgressBytes += row.Size
		if row.Status >= 400 {
			m.Instance.ErrorRequests++
		}
		m.TrafficDuration += row.Duration
	}
	if m.Instance.Requests > 0 {
		m.Instance.LatencyMS = m.TrafficDuration / float64(m.Instance.Requests) * 1000
	}
}

func (s *server) logs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mutex.Lock()
	_, err := s.load(id)
	s.mutex.Unlock()
	if err != nil {
		http.Error(w, "unknown managed instance", 404)
		return
	}
	output, err := command("journalctl", "--unit", s.unit(id), "--lines=100", "--no-pager", "--output=short-iso")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	writeJSON(w, lines)
}
