package manager

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPTokenScopeTasksAndRevocation(t *testing.T) {
	m, handler, cookie := setup(t)
	for _, name := range []string{"allowed-db", "other-db"} {
		response := perform(handler, "POST", "/api/manager/instances", `{"name":"`+name+`"}`, cookie)
		if response.Code != 202 || !m.work() {
			t.Fatal("instance creation failed")
		}
	}
	items, _ := instances(m.app)
	allowed, other := items[0].ID, items[1].ID
	body, _ := json.Marshal(map[string]any{"name": "test-agent", "instanceIds": []string{allowed}})
	created := perform(handler, "POST", "/api/manager/agent-tokens", string(body), cookie)
	if created.Code != 201 {
		t.Fatalf("agent creation: %s", created.Body.String())
	}
	var credential struct {
		Token string     `json:"token"`
		Agent AgentToken `json:"agent"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &credential)
	if credential.Token == "" {
		t.Fatal("missing token")
	}
	rpc := func(token, method string, params any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		request := httptest.NewRequest("POST", "http://manager.example.com/mcp", strings.NewReader(string(body)))
		request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8095}))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("MCP-Protocol-Version", "2025-06-18")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := rpc("", "tools/list", map[string]any{}); response.Code != 401 {
		t.Fatal("anonymous MCP access allowed")
	}
	invalidHost := httptest.NewRequest("POST", "http://attacker.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	invalidHost.Header.Set("Authorization", "Bearer "+credential.Token)
	invalidHost.Header.Set("Content-Type", "application/json")
	blockedHost := httptest.NewRecorder()
	handler.ServeHTTP(blockedHost, invalidHost)
	if blockedHost.Code != 403 {
		t.Fatal("unconfigured MCP hostname was accepted")
	}
	initialized := rpc(credential.Token, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	if initialized.Code != 200 || !strings.Contains(initialized.Body.String(), "PocketBase Manager") {
		t.Fatalf("initialize: %d %s", initialized.Code, initialized.Body.String())
	}
	callTool := func(name string, args map[string]any) string {
		response := rpc(credential.Token, "tools/call", map[string]any{"name": name, "arguments": args})
		if response.Code != 200 {
			t.Fatalf("tool HTTP %d: %s", response.Code, response.Body.String())
		}
		return response.Body.String()
	}
	listing := callTool("list_instances", map[string]any{})
	if !strings.Contains(listing, allowed) || strings.Contains(listing, other) {
		t.Fatalf("instance scope leaked: %s", listing)
	}
	blocked := callTool("pocketbase_request", map[string]any{"instanceId": other, "method": "GET", "path": "/api/collections"})
	if !strings.Contains(blocked, `"isError":true`) {
		t.Fatalf("other instance was allowed: %s", blocked)
	}
	result := callTool("pocketbase_request", map[string]any{"instanceId": allowed, "method": "POST", "path": "/api/collections/posts/records", "body": map[string]any{"title": "Hello"}})
	if strings.Contains(result, `"isError":true`) || len(m.agent.(*fakeAgent).dbCalls) != 1 {
		t.Fatalf("database forwarding failed: %s", result)
	}
	result = callTool("create_task", map[string]any{"title": "Test task", "instanceId": allowed})
	if strings.Contains(result, `"isError":true`) {
		t.Fatal(result)
	}
	tasks, err := m.agentTasks(credential.Agent, "", "")
	if err != nil || len(tasks) != 1 {
		t.Fatal("task was not persisted")
	}
	result = callTool("update_task", map[string]any{"id": tasks[0].ID, "status": "done"})
	if strings.Contains(result, `"isError":true`) {
		t.Fatal(result)
	}
	record, _ := m.app.FindRecordById("manager_agent_tokens", credential.Agent.ID)
	if record.GetString("hash") == credential.Token || strings.Contains(record.GetString("data"), credential.Token) {
		t.Fatal("raw agent token was persisted")
	}
	response := perform(handler, "DELETE", "/api/manager/agent-tokens/"+credential.Agent.ID, "", cookie)
	if response.Code != 204 {
		t.Fatal("revocation failed")
	}
	if response := rpc(credential.Token, "tools/list", map[string]any{}); response.Code != 401 {
		t.Fatal("revoked token remained valid")
	}
}
