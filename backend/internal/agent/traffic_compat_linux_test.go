package agent

import (
	"os"
	"path/filepath"
	"testing"

	"pbmanager/backend/internal/platform"
)

func TestOlderCaddyContentLengthIsMarkedAsEstimated(t *testing.T) {
	dir := t.TempDir()
	s := &server{config: config{LogDir: dir}}
	m := &manifest{Instance: platform.Instance{ID: "abcdefghijklmno"}}
	lines := `{"request":{"headers":{"Content-Length":["2109"]}},"status":400,"size":223}` + "\n" + `{"request":{"headers":{"Content-Length":["9999"]}},"status":413,"size":10}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "pbm-"+m.Instance.ID+".json"), []byte(lines), 0600); err != nil {
		t.Fatal(err)
	}
	s.traffic(m)
	if m.Instance.IngressBytes != 2109 || !m.Instance.IngressEstimated || m.Instance.Requests != 2 {
		t.Fatalf("older Caddy traffic was misreported: %+v", m.Instance)
	}
}
