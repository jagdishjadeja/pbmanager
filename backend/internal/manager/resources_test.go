package manager

import (
	"strings"
	"testing"

	"pbmanager/backend/internal/platform"
)

func TestRAMAndFileAccountOperationsPreserveStoppedState(t *testing.T) {
	m, handler, cookie := setup(t)
	response := perform(handler, "POST", "/api/manager/instances", `{"name":"resource-demo","memoryLimitMiB":768}`, cookie)
	if response.Code != 202 || !m.work() {
		t.Fatal("create failed")
	}
	items, _ := instances(m.app)
	id := items[0].ID
	if items[0].MemoryLimitBytes != 768*platform.MiB {
		t.Fatal("RAM choice not saved")
	}
	response = perform(handler, "POST", "/api/manager/instances/"+id+"/actions/stop", "", cookie)
	if response.Code != 202 || !m.work() {
		t.Fatal("stop failed")
	}
	response = perform(handler, "PATCH", "/api/manager/instances/"+id+"/resources", `{"memoryLimitMiB":1024}`, cookie)
	if response.Code != 202 || !m.work() {
		t.Fatal("RAM update failed")
	}
	item, _ := m.find(id)
	if item.Status != "stopped" || item.MemoryLimitBytes != 1024*platform.MiB {
		t.Fatal("RAM change started a stopped instance")
	}
	response = perform(handler, "PATCH", "/api/manager/instances/"+id+"/resources", `{"memoryLimitMiB":129}`, cookie)
	if response.Code != 400 {
		t.Fatal("invalid RAM accepted")
	}
	password := "test-only-file-password"
	response = perform(handler, "POST", "/api/manager/instances/"+id+"/sftp-users", `{"username":"abc123","password":"`+password+`"}`, cookie)
	if response.Code != 202 {
		t.Fatalf("file account: %d %s", response.Code, response.Body.String())
	}
	records, _ := m.app.FindRecordsByFilter("manager_operations", "state='queued'", "", 1, 0)
	if len(records) != 1 || strings.Contains(records[0].GetString("secret"), password) {
		t.Fatal("password was not encrypted")
	}
	if !m.work() {
		t.Fatal("file account operation failed")
	}
	item, _ = m.find(id)
	if item.Status != "stopped" || len(item.SFTPUsers) != 1 {
		t.Fatal("file account changed instance state")
	}
	record, _ := m.app.FindRecordById("manager_operations", records[0].Id)
	if record.GetString("secret") != "" {
		t.Fatal("consumed password was retained")
	}
}
