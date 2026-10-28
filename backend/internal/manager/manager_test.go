package manager

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
)

type fakeAgent struct {
	items   map[string]platform.Instance
	calls   []platform.AgentRequest
	dbCalls []platform.DatabaseRequest
}

func (f *fakeAgent) Execute(request platform.AgentRequest) error {
	f.calls = append(f.calls, request)
	i := request.Instance
	if request.Action == "delete" {
		delete(f.items, i.ID)
	} else {
		if request.Action == "stop" {
			i.Status = "stopped"
		} else if request.Action == "create" || request.Action == "start" || request.Action == "restart" {
			i.Status = "running"
		}
		if request.Action == "sftp-create" {
			i.SFTPUsers = append(i.SFTPUsers, platform.SFTPUser{Username: request.Username, CreatedAt: platform.Now()})
		}
		i.DatabaseBytes = 4096
		i.StorageBytes = 8192
		f.items[i.ID] = i
	}
	return nil
}
func (f *fakeAgent) Snapshot() (platform.AgentSnapshot, error) {
	snapshot := platform.AgentSnapshot{Instances: []platform.Instance{}, Storage: platform.Storage{TotalBytes: 40 << 30, AvailableBytes: 30 << 30, UsedBytes: 10 << 30, MeasuredAt: platform.Now()}, Server: platform.Server{Name: "test-server", BaseDomain: "pb.example.com"}}
	for _, i := range f.items {
		snapshot.Instances = append(snapshot.Instances, i)
	}
	return snapshot, nil
}
func (f *fakeAgent) Logs(string) ([]string, error) { return []string{"test log"}, nil }
func (f *fakeAgent) Database(input platform.DatabaseRequest) (platform.DatabaseResponse, error) {
	f.dbCalls = append(f.dbCalls, input)
	return platform.DatabaseResponse{Status: 200, Body: json.RawMessage(`{"ok":true}`)}, nil
}

func setup(t *testing.T) (*Manager, http.Handler, *http.Cookie) {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	collection, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	admin := core.NewRecord(collection)
	admin.SetEmail("admin@example.com")
	admin.SetPassword("test-only-password-long-enough")
	if err = app.Save(admin); err != nil {
		t.Fatal(err)
	}
	m, err := New(app, &fakeAgent{items: map[string]platform.Instance{}}, Config{Key: bytes.Repeat([]byte{1}, 32), BaseDomain: "pb.example.com", ManagerHost: "manager.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	m.Register(router)
	handler, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	login := perform(handler, "POST", "/api/manager/session", `{"email":"admin@example.com","password":"test-only-password-long-enough"}`, nil)
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	return m, handler, cookie
}

func perform(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestAuthorizationAndOrigin(t *testing.T) {
	_, handler, cookie := setup(t)
	for _, path := range []string{"/api/manager/snapshot", "/api/manager/status", "/api/manager/usage"} {
		if response := perform(handler, "GET", path, "", nil); response.Code != 401 {
			t.Fatalf("%s allowed unauthenticated: %d", path, response.Code)
		}
	}
	if response := perform(handler, "GET", "/api/manager/session", "", cookie); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	request := httptest.NewRequest("POST", "http://manager.example.com/api/manager/instances", strings.NewReader(`{"name":"blocked"}`))
	request.Header.Set("Origin", "https://evil.example.com")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("cross-origin request allowed: %d", response.Code)
	}
}

func TestProvisionLifecycleAndReusableCredentials(t *testing.T) {
	m, handler, cookie := setup(t)
	response := perform(handler, "POST", "/api/manager/instances", `{"name":""}`, cookie)
	if response.Code != 202 {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Instance  platform.Instance  `json:"instance"`
		Operation platform.Operation `json:"operation"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[a-z0-9]{6}$`).MatchString(result.Instance.Name) {
		t.Fatalf("invalid generated name: %s", result.Instance.Name)
	}
	duplicate := perform(handler, "POST", "/api/manager/instances", `{"name":"`+result.Instance.Name+`"}`, cookie)
	if duplicate.Code != 409 {
		t.Fatalf("duplicate not rejected: %d", duplicate.Code)
	}
	if !m.work() {
		t.Fatal("no queued operation")
	}
	operation := perform(handler, "GET", "/api/manager/operations/"+result.Operation.ID, "", cookie)
	if !strings.Contains(operation.Body.String(), `"succeeded"`) {
		t.Fatal(operation.Body.String())
	}
	credentialPath := "/api/manager/instances/" + result.Instance.ID + "/credentials"
	if response := perform(handler, "POST", credentialPath, "", nil); response.Code != 401 {
		t.Fatalf("credentials allowed unauthenticated: %d", response.Code)
	}
	credentials := perform(handler, "POST", credentialPath, "", cookie)
	var login struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(credentials.Body.Bytes(), &login); err != nil || credentials.Code != 200 || login.Email == "" || login.Password == "" {
		t.Fatalf("credentials unavailable: status %d, error %v", credentials.Code, err)
	}
	request := m.agent.(*fakeAgent).calls[0]
	if login.Email != request.Email || login.Password != request.Password {
		t.Fatal("credentials do not match the provisioned superadmin")
	}
	again := perform(handler, "POST", credentialPath, "", cookie)
	if again.Code != 200 || !bytes.Equal(again.Body.Bytes(), credentials.Body.Bytes()) {
		t.Fatal("credentials changed or became unavailable after the first view")
	}
	stored, err := m.app.FindRecordById("manager_operations", result.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secret := stored.GetString("secret"); secret == "" || strings.Contains(secret, login.Password) {
		t.Fatal("credentials were not retained encrypted")
	}
	stop := perform(handler, "POST", "/api/manager/instances/"+result.Instance.ID+"/actions/stop", "", cookie)
	if stop.Code != 202 {
		t.Fatal(stop.Body.String())
	}
	m.work()
	item, err := m.find(result.Instance.ID)
	if err != nil || item.Status != "stopped" {
		t.Fatalf("stop failed: %v %s", err, item.Status)
	}
	again = perform(handler, "POST", credentialPath, "", cookie)
	if again.Code != 200 || !bytes.Equal(again.Body.Bytes(), credentials.Body.Bytes()) {
		t.Fatal("credentials became unavailable after stopping the instance")
	}
	deleted := perform(handler, "DELETE", "/api/manager/instances/"+result.Instance.ID, "", cookie)
	if deleted.Code != 202 {
		t.Fatal(deleted.Body.String())
	}
	m.work()
	if _, err = m.find(result.Instance.ID); err == nil {
		t.Fatal("instance was not deleted")
	}
	if response := perform(handler, "POST", credentialPath, "", cookie); response.Code != 404 {
		t.Fatalf("credentials available after instance deletion: %d", response.Code)
	}
	stored, err = m.app.FindRecordById("manager_operations", result.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetString("secret") != "" {
		t.Fatal("credentials were retained after instance deletion")
	}
}

func TestConcurrentOperationAndInvalidInput(t *testing.T) {
	m, handler, cookie := setup(t)
	for _, name := range []string{"../root", "ADMIN", "www", "ab", "name;touch"} {
		payload, _ := json.Marshal(map[string]string{"name": name})
		response := perform(handler, "POST", "/api/manager/instances", string(payload), cookie)
		if response.Code != 400 {
			t.Fatalf("invalid name accepted: %s, %d", name, response.Code)
		}
	}
	response := perform(handler, "POST", "/api/manager/instances", `{"name":"gronda"}`, cookie)
	if response.Code != 202 {
		t.Fatal(response.Body.String())
	}
	var result struct {
		Instance platform.Instance `json:"instance"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &result)
	conflict := perform(handler, "DELETE", "/api/manager/instances/"+result.Instance.ID, "", cookie)
	if conflict.Code != 409 {
		t.Fatal("parallel operation was accepted")
	}
	m.work()
	snapshot := perform(handler, "GET", "/api/manager/snapshot", "", cookie)
	if !strings.Contains(snapshot.Body.String(), "test-server") {
		t.Fatal(snapshot.Body.String())
	}
}
