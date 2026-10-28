package manager

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"pbmanager/backend/internal/platform"
	"pbmanager/backend/internal/staticfiles"
)

const sessionCookie = "pbmanager_session"

func (m *Manager) Register(r *router.Router[*core.RequestEvent]) {
	m.registerMCP(r)
	group := r.Group("/api/manager")
	group.BindFunc(func(e *core.RequestEvent) error {
		e.Response.Header().Set("Cache-Control", "no-store")
		if e.Request.Method != "GET" && e.Request.Method != "HEAD" {
			if origin := e.Request.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Host != e.Request.Host {
					return e.ForbiddenError("Request origin is not allowed.", nil)
				}
			}
		}
		return e.Next()
	})
	group.POST("/session", m.login)
	group.GET("/session", m.session).BindFunc(m.auth)
	group.DELETE("/session", m.logout).BindFunc(m.auth)
	private := group.Group("")
	private.BindFunc(m.auth)
	private.GET("/status", func(e *core.RequestEvent) error {
		return e.JSON(200, map[string]any{"name": "PocketBase Manager", "mode": "live", "provisioning": true, "reverseProxy": "caddy", "baseDomain": m.config.BaseDomain})
	})
	private.GET("/snapshot", func(e *core.RequestEvent) error {
		m.cache.RLock()
		defer m.cache.RUnlock()
		return e.JSON(200, m.snapshot)
	})
	private.POST("/refresh", func(e *core.RequestEvent) error {
		m.refresh()
		m.cache.RLock()
		defer m.cache.RUnlock()
		return e.JSON(200, m.snapshot)
	})
	private.POST("/instances", m.create)
	private.PATCH("/instances/{id}/resources", m.updateResources)
	private.POST("/instances/{id}/actions/{action}", m.control)
	private.DELETE("/instances/{id}", m.remove)
	private.GET("/instances/{id}/logs", m.logs)
	private.GET("/usage", m.usage)
	private.GET("/operations/{id}", m.operation)
	private.POST("/instances/{id}/credentials", m.credentials)
	private.POST("/instances/{id}/domains", m.addDomain)
	private.DELETE("/instances/{id}/domains/{hostname}", m.removeDomain)
	private.POST("/instances/{id}/domains/{hostname}/verify", m.verifyDomain)
	private.POST("/instances/{id}/site", m.uploadSite).Bind(apis.BodyLimit(staticfiles.MaxUploadBytes + (1 << 20)))
	private.DELETE("/instances/{id}/site", m.removeSite)
	private.GET("/agent-tokens", m.listAgentTokens)
	private.POST("/agent-tokens", m.createAgentToken)
	private.DELETE("/agent-tokens/{id}", m.revokeAgentToken)
	private.GET("/agent-tasks", m.listAgentTasks)
	private.DELETE("/agent-tasks/{id}", m.deleteAgentTask)
	private.POST("/instances/{id}/sftp-users", m.createSFTPUser)
	private.DELETE("/instances/{id}/sftp-users/{username}", m.removeSFTPUser)
}

func (m *Manager) auth(e *core.RequestEvent) error {
	if e.Auth != nil && e.Auth.IsSuperuser() {
		return e.Next()
	}
	cookie, err := e.Request.Cookie(sessionCookie)
	if err != nil {
		return e.UnauthorizedError("Sign in to manage instances.", nil)
	}
	record, err := m.app.FindAuthRecordByToken(cookie.Value, core.TokenTypeAuth)
	if err != nil || !record.IsSuperuser() {
		return e.UnauthorizedError("Your session has expired. Sign in again.", nil)
	}
	e.Auth = record
	return e.Next()
}

func (m *Manager) login(e *core.RequestEvent) error {
	address := e.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	// Caddy is the only upstream of the loopback-bound manager.
	if ip := net.ParseIP(address); ip != nil && ip.IsLoopback() {
		if forwarded := e.Request.Header.Get("X-Forwarded-For"); forwarded != "" {
			address = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
	}
	m.loginMutex.Lock()
	now := time.Now()
	recent := []time.Time{}
	for _, at := range m.loginAttempts[address] {
		if now.Sub(at) < 10*time.Minute {
			recent = append(recent, at)
		}
	}
	if len(recent) >= 10 {
		m.loginMutex.Unlock()
		return e.Error(429, "Too many sign-in attempts. Try again in ten minutes.", nil)
	}
	m.loginAttempts[address] = append(recent, now)
	m.loginMutex.Unlock()
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, 8192)
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("Enter your email and password.", nil)
	}
	record, err := m.app.FindAuthRecordByEmail(core.CollectionNameSuperusers, strings.TrimSpace(body.Email))
	if err != nil || !record.ValidatePassword(body.Password) {
		return e.UnauthorizedError("Email or password is incorrect.", nil)
	}
	token, err := record.NewAuthToken()
	if err != nil {
		return e.InternalServerError("Could not create session.", err)
	}
	http.SetCookie(e.Response, &http.Cookie{Name: sessionCookie, Value: token, Path: "/api/manager", HttpOnly: true, Secure: m.config.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	m.loginMutex.Lock()
	delete(m.loginAttempts, address)
	m.loginMutex.Unlock()
	return e.JSON(200, map[string]any{"email": record.Email(), "baseDomain": m.config.BaseDomain})
}
func (m *Manager) session(e *core.RequestEvent) error {
	return e.JSON(200, map[string]any{"email": e.Auth.Email(), "baseDomain": m.config.BaseDomain})
}
func (m *Manager) logout(e *core.RequestEvent) error {
	http.SetCookie(e.Response, &http.Cookie{Name: sessionCookie, Value: "", Path: "/api/manager", HttpOnly: true, Secure: m.config.SecureCookies, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	return e.NoContent(204)
}

func (m *Manager) find(id string) (platform.Instance, error) {
	record, err := m.app.FindRecordById("manager_instances", id)
	if err != nil {
		return platform.Instance{}, err
	}
	item, err := decode[platform.Instance](record)
	item.ID = record.Id
	platform.EnsureDomains(&item)
	return item, err
}

func (m *Manager) create(e *core.RequestEvent) error {
	var body struct {
		Name           string `json:"name"`
		CustomDomain   string `json:"customDomain"`
		MemoryLimitMiB int    `json:"memoryLimitMiB"`
	}
	e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, 4096)
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("Invalid instance request.", nil)
	}
	if body.MemoryLimitMiB == 0 {
		body.MemoryLimitMiB = platform.DefaultMemoryMiB
	}
	limit, limitErr := platform.MemoryLimit(body.MemoryLimitMiB)
	if limitErr != nil {
		return e.BadRequestError(limitErr.Error(), nil)
	}
	m.mutation.Lock()
	defer m.mutation.Unlock()
	items, err := instances(m.app)
	if err != nil {
		return e.InternalServerError("Could not read instances.", err)
	}
	used := map[string]bool{}
	ports := map[int]bool{}
	for _, item := range items {
		used[item.Name] = true
		ports[item.Port] = true
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		for {
			name = platform.RandomName()
			if !used[name] {
				break
			}
		}
	}
	if err := platform.ValidateName(name); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	if name+"."+m.config.BaseDomain == m.config.ManagerHost {
		return e.BadRequestError("This hostname is reserved for the manager.", nil)
	}
	if used[name] {
		return e.Error(409, "This instance name is already in use.", nil)
	}
	port := 21000
	for ports[port] && port <= 29999 {
		port++
	}
	if port > 29999 {
		return e.Error(409, "No available instance ports.", nil)
	}
	instance := platform.Instance{Name: name, Hostname: name + "." + m.config.BaseDomain, Port: port, Status: "provisioning", Version: "0.39.0", QuotaBytes: 1 << 30, MemoryLimitBytes: limit, CreatedAt: platform.Now(), UpdatedAt: platform.Now()}
	platform.EnsureDomains(&instance)
	instance.ProvisioningStage = "queued"
	instance.ProvisioningMessage = "Waiting for provisioning agent"
	if strings.TrimSpace(body.CustomDomain) != "" {
		domain, err := m.customDomain(body.CustomDomain, "")
		if err != nil {
			return e.BadRequestError(err.Error(), nil)
		}
		instance.Domains = append(instance.Domains, domain)
	}
	record, err := put(m.app, "manager_instances", "", instance, map[string]any{"name": name, "port": port})
	if err != nil {
		return e.InternalServerError("Could not reserve instance.", err)
	}
	instance.ID = record.Id
	if err = saveInstance(m.app, instance); err != nil {
		_ = m.app.Delete(record)
		return e.InternalServerError("Could not save instance.", err)
	}
	request := platform.AgentRequest{Instance: instance, Action: "create", Email: "admin@" + instance.Hostname, Password: platform.RandomString(36)}
	operation, err := m.enqueue(instance, "create", request)
	if err != nil {
		_ = m.app.Delete(record)
		return e.Error(409, err.Error(), nil)
	}
	return e.JSON(202, map[string]any{"instance": instance, "operation": operation})
}

func (m *Manager) control(e *core.RequestEvent) error {
	action := e.Request.PathValue("action")
	if action != "start" && action != "stop" && action != "restart" {
		return e.BadRequestError("Unsupported service action.", nil)
	}
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.find(e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("Instance not found.", nil)
	}
	request := platform.AgentRequest{Instance: i, Action: action}
	if i.Status == "failed" && action != "stop" {
		failed, _ := m.app.FindRecordsByFilter("manager_operations", "instance={:id} && state='failed'", "-created", 1, 0, dbx.Params{"id": i.ID})
		if len(failed) > 0 {
			previous, _ := decode[platform.Operation](failed[0])
			if previous.Action == "create" {
				if err = unseal(m.config.Key, failed[0].GetString("secret"), &request); err != nil {
					return e.InternalServerError("Could not resume provisioning.", err)
				}
				action = "create"
			}
		}
	}
	operation, err := m.enqueue(i, action, request)
	if err != nil {
		return e.Error(409, err.Error(), nil)
	}
	return e.JSON(202, operation)
}

func (m *Manager) remove(e *core.RequestEvent) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.find(e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("Instance not found.", nil)
	}
	operation, err := m.enqueue(i, "delete", platform.AgentRequest{Instance: i, Action: "delete"})
	if err != nil {
		return e.Error(409, err.Error(), nil)
	}
	return e.JSON(202, operation)
}
func (m *Manager) operation(e *core.RequestEvent) error {
	record, err := m.app.FindRecordById("manager_operations", e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("Operation not found.", nil)
	}
	operation, err := decode[platform.Operation](record)
	if err != nil {
		return e.InternalServerError("Invalid operation.", err)
	}
	return e.JSON(200, operation)
}
func (m *Manager) logs(e *core.RequestEvent) error {
	if _, err := m.find(e.Request.PathValue("id")); err != nil {
		return e.NotFoundError("Instance not found.", nil)
	}
	lines, err := m.agent.Logs(e.Request.PathValue("id"))
	if err != nil {
		return e.Error(502, err.Error(), nil)
	}
	return e.JSON(200, lines)
}

func (m *Manager) credentials(e *core.RequestEvent) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.find(e.Request.PathValue("id"))
	if err != nil {
		return e.NotFoundError("Instance not found.", nil)
	}
	records, err := m.app.FindRecordsByFilter("manager_operations", "instance={:id} && state='succeeded'", "-created", 0, 0, dbx.Params{"id": i.ID})
	if err != nil {
		return e.InternalServerError("Could not read initial credentials.", err)
	}
	for _, record := range records {
		operation, _ := decode[platform.Operation](record)
		if operation.Action != "create" || record.GetString("secret") == "" {
			continue
		}
		var request platform.AgentRequest
		if err = unseal(m.config.Key, record.GetString("secret"), &request); err != nil {
			return e.InternalServerError("Could not decrypt credentials.", err)
		}
		return e.JSON(200, map[string]string{"email": request.Email, "password": request.Password})
	}
	return e.NotFoundError("Initial superadmin credentials are unavailable.", nil)
}

func (m *Manager) usage(e *core.RequestEvent) error {
	hours, err := strconv.Atoi(e.Request.URL.Query().Get("hours"))
	if err != nil || hours < 1 {
		hours = 24
	}
	if hours > 720 {
		hours = 720
	}
	id := e.Request.URL.Query().Get("id")
	if id != "" {
		if _, err = m.find(id); err != nil {
			return e.NotFoundError("Instance not found.", nil)
		}
	}
	filter := "time>={:start}"
	params := dbx.Params{"start": time.Now().Add(-time.Duration(hours) * time.Hour).Unix()}
	if id != "" {
		filter += " && instance={:id}"
		params["id"] = id
	}
	records, err := m.app.FindRecordsByFilter("manager_metrics", filter, "time", 0, 0, params)
	if err != nil {
		return e.InternalServerError("Could not read usage history.", err)
	}
	buckets := map[int64]*platform.Sample{}
	bucketWidth := max(int64(60), int64(hours)*3600/48)
	for _, record := range records {
		sample, err := decode[platform.Sample](record)
		if err != nil {
			continue
		}
		bucket := int64(record.GetInt("time")) / bucketWidth * bucketWidth
		current := buckets[bucket]
		if current == nil {
			current = &platform.Sample{At: time.Unix(bucket, 0).UTC().Format(time.RFC3339)}
			buckets[bucket] = current
		}
		current.Requests += sample.Requests
		current.IngressBytes += sample.IngressBytes
		current.EgressBytes += sample.EgressBytes
		current.ErrorRequests += sample.ErrorRequests
		current.CPUPercent += sample.CPUPercent
		current.MemoryBytes += sample.MemoryBytes
		current.LatencyMS = sample.LatencyMS
	}
	result := []platform.Sample{}
	start := time.Now().Add(-time.Duration(hours)*time.Hour).Unix() / bucketWidth * bucketWidth
	end := time.Now().Unix() / bucketWidth * bucketWidth
	for at := start; at <= end; at += bucketWidth {
		if sample := buckets[at]; sample != nil {
			result = append(result, *sample)
		}
	}
	return e.JSON(200, result)
}
