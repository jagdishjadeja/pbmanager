package agent

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"pbmanager/backend/internal/platform"
)

func TestCaddySnippetModeIgnoresRestrictiveUmask(t *testing.T) {
	old := syscall.Umask(0077)
	defer syscall.Umask(old)
	path := filepath.Join(t.TempDir(), "instance.caddy")
	if err := atomicWrite(path, []byte("example.com {}"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("Caddy cannot read mode %o", info.Mode().Perm())
	}
}

func TestTrafficAccountingAcrossPartialWritesAndRotation(t *testing.T) {
	dir := t.TempDir()
	s := &server{config: config{LogDir: dir}}
	m := &manifest{Instance: platform.Instance{ID: "abcdefghijklmno"}}
	path := filepath.Join(dir, "pbm-"+m.Instance.ID+".json")
	line := []byte("{\"bytes_read\":12,\"size\":24,\"status\":200,\"duration\":0.01}\n")
	if err := os.WriteFile(path, line, 0600); err != nil {
		t.Fatal(err)
	}
	s.traffic(m)
	s.traffic(m)
	if m.Instance.Requests != 1 {
		t.Fatal("re-reading logs duplicated requests")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write(line)
	file.Close()
	if err = os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	partial := line[:len(line)-1]
	if err = os.WriteFile(path, partial, 0600); err != nil {
		t.Fatal(err)
	}
	s.traffic(m)
	if m.Instance.Requests != 2 {
		t.Fatal("unread tail was lost during rotation")
	}
	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("\n"))
	file.Close()
	s.traffic(m)
	if m.Instance.Requests != 3 || m.Instance.IngressBytes != 36 || m.Instance.EgressBytes != 72 {
		t.Fatalf("incorrect traffic counters: %+v", m.Instance)
	}
}
