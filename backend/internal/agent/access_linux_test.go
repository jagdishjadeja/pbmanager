package agent

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pbmanager/backend/internal/platform"
)

func TestPublicPointerSwitchForScopedFileAccess(t *testing.T) {
	s := &server{config: config{Root: t.TempDir()}}
	id := "abcdefghijklmno"
	for _, name := range []string{"first", "second"} {
		root := filepath.Join(s.path(id), "site", ".releases", ".site-release-"+name)
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"first", "second"} {
		target := filepath.Join("site", ".releases", ".site-release-"+name)
		if err := s.switchPublic(id, target); err != nil {
			t.Fatal(err)
		}
		actual, err := s.publicTarget(id)
		if err != nil || actual != target {
			t.Fatalf("wrong release: %s %v", actual, err)
		}
		data, err := os.ReadFile(filepath.Join(s.path(id), "pb_public", "index.html"))
		if err != nil || string(data) != name {
			t.Fatal("public directory does not follow the current release")
		}
	}
	_ = os.Remove(filepath.Join(s.path(id), "site", "current"))
	_ = os.Symlink("../../pb_data", filepath.Join(s.path(id), "site", "current"))
	if _, err := s.publicTarget(id); err == nil {
		t.Fatal("unsafe public target was trusted")
	}
}

func TestDatabaseProxyRejectsExternalAndTraversalPaths(t *testing.T) {
	s := &server{}
	for _, path := range []string{"http://127.0.0.1:8095/api/settings", "//example.com/api/settings", "/api/../outside", "/api/%2e%2e/outside", "/api/collections\\outside", "/_/"} {
		body, _ := json.Marshal(platform.DatabaseRequest{InstanceID: "abcdefghijklmno", Method: "GET", Path: path})
		request := httptest.NewRequest("POST", "/database", bytes.NewReader(body))
		response := httptest.NewRecorder()
		s.databaseRequest(response, request)
		if response.Code != 400 {
			t.Fatalf("unsafe path accepted: %s -> %d", path, response.Code)
		}
	}
}
