package agent

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"pbmanager/backend/internal/platform"
	"pbmanager/backend/internal/staticfiles"
)

func (s *server) uploadSite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !platform.ValidateID(id) {
		http.Error(w, "invalid instance", 400)
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	m, err := s.load(id)
	if err != nil {
		http.Error(w, "unknown managed instance", 404)
		return
	}
	// Staging and extraction must stay inside the instance's bounded volume.
	if _, err = command("mountpoint", "-q", s.path(id)); err != nil {
		http.Error(w, "instance storage is not mounted", 409)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, staticfiles.MaxUploadBytes)
	archive, err := os.CreateTemp(s.path(id), ".site-upload-")
	if err != nil {
		http.Error(w, "could not stage upload", 500)
		return
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	if _, err = io.Copy(archive, r.Body); err != nil {
		http.Error(w, "upload failed or exceeds 50 MiB", 413)
		return
	}
	stageParent := s.path(id)
	if _, err := os.Stat(filepath.Join(s.path(id), "site", ".releases")); err == nil {
		stageParent = filepath.Join(s.path(id), "site", ".releases")
	}
	stage, err := os.MkdirTemp(stageParent, ".site-release-")
	if err != nil {
		http.Error(w, "could not stage site", 500)
		return
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	summary, err := staticfiles.Extract(archive, r.Header.Get("X-Site-Format"), stage)
	if err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	if err = s.sitePermissions(stage, id); err != nil {
		http.Error(w, "could not prepare public files", 500)
		return
	}
	old, err := s.publicTarget(id)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	if err = s.configurePublicDir(m.Instance); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	relativeStage, _ := filepath.Rel(s.path(id), stage)
	if err = s.switchPublic(id, relativeStage); err != nil {
		http.Error(w, "could not publish site", 500)
		return
	}
	name := filepath.Base(strings.ReplaceAll(r.Header.Get("X-Site-Filename"), "\\", "/"))
	if len(name) > 160 {
		name = name[:160]
	}
	previous := m.Instance.StaticSite
	m.Instance.StaticSite = &platform.StaticSite{Filename: name, Files: summary.Files, Bytes: summary.Bytes, PublishedAt: platform.Now()}
	if err = s.save(m); err != nil {
		_ = s.switchPublic(id, old)
		m.Instance.StaticSite = previous
		http.Error(w, "could not save site deployment", 500)
		return
	}
	keep = true
	if old != "" {
		_ = os.RemoveAll(filepath.Join(s.path(id), old))
	}
	writeJSON(w, m.Instance.StaticSite)
}

// Only links created by this manager can be replaced or removed.
func (s *server) publicTarget(id string) (string, error) {
	path := filepath.Join(s.path(id), "pb_public")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", errors.New("an unmanaged pb_public directory already exists; move it before publishing")
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	if target == "site/current" {
		current, err := os.Readlink(filepath.Join(s.path(id), "site", "current"))
		if err != nil {
			return "", err
		}
		if filepath.Dir(current) != ".releases" || !strings.HasPrefix(filepath.Base(current), ".site-release-") {
			return "", errors.New("unrecognized static release directory")
		}
		return filepath.Join("site", current), nil
	}
	if filepath.Base(target) != target || !strings.HasPrefix(target, ".site-release-") {
		return "", errors.New("unrecognized static site directory")
	}
	return target, nil
}

func (s *server) switchPublic(id, target string) error {
	public := filepath.Join(s.path(id), "pb_public")
	if target == "" {
		err := os.Remove(public)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if strings.HasPrefix(target, "site/.releases/") {
		next := filepath.Join(s.path(id), "site", ".current-"+platform.RandomString(12))
		if err := os.Symlink(filepath.Join(".releases", filepath.Base(target)), next); err != nil {
			return err
		}
		defer os.Remove(next)
		if err := os.Rename(next, filepath.Join(s.path(id), "site", "current")); err != nil {
			return err
		}
		target = "site/current"
	}
	temporary := filepath.Join(s.path(id), ".site-link-"+platform.RandomString(12))
	if err := os.Symlink(target, temporary); err != nil {
		return err
	}
	defer os.Remove(temporary)
	return os.Rename(temporary, public)
}

func (s *server) deleteSite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !platform.ValidateID(id) {
		http.Error(w, "invalid instance", 400)
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	m, err := s.load(id)
	if err != nil {
		http.Error(w, "unknown managed instance", 404)
		return
	}
	if _, err = command("mountpoint", "-q", s.path(id)); err != nil {
		http.Error(w, "instance storage is not mounted", 409)
		return
	}
	target, err := s.publicTarget(id)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	next := ""
	if len(m.Instance.SFTPUsers) > 0 {
		empty, createErr := os.MkdirTemp(filepath.Join(s.path(id), "site", ".releases"), ".site-release-")
		if createErr != nil {
			http.Error(w, "could not reset website folder", 500)
			return
		}
		if err = s.sitePermissions(empty, id); err != nil {
			_ = os.RemoveAll(empty)
			http.Error(w, "could not prepare empty website folder", 500)
			return
		}
		next, _ = filepath.Rel(s.path(id), empty)
	}
	if err = s.switchPublic(id, next); err != nil {
		http.Error(w, "could not remove public site", 500)
		return
	}
	m.Instance.StaticSite = nil
	if err = s.save(m); err != nil {
		_ = s.switchPublic(id, target)
		http.Error(w, "could not save site removal", 500)
		return
	}
	if target != "" {
		_ = os.RemoveAll(filepath.Join(s.path(id), target))
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) configurePublicDir(i platform.Instance) error {
	path := filepath.Join("/etc/systemd/system", s.unit(i.ID))
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	expected := "--publicDir=" + filepath.Join(s.path(i.ID), "pb_public")
	for _, line := range strings.Split(string(original), "\n") {
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if field == expected {
				return nil
			}
			if strings.HasPrefix(field, "--publicDir") {
				return errors.New("instance has an unexpected public directory configuration")
			}
		}
		updated := strings.Replace(string(original), line, line+" "+expected, 1)
		if err = atomicWrite(path, []byte(updated), 0644); err != nil {
			return err
		}
		_, err = command("systemctl", "daemon-reload")
		if err == nil {
			if _, activeErr := command("systemctl", "is-active", "--quiet", s.unit(i.ID)); activeErr == nil {
				_, err = command("systemctl", "restart", s.unit(i.ID))
				if err == nil {
					err = s.healthy(i)
				}
			}
		}
		if err != nil {
			_ = atomicWrite(path, original, 0644)
			_, _ = command("systemctl", "daemon-reload")
			_, _ = command("systemctl", "try-restart", s.unit(i.ID))
		}
		return err
	}
	return errors.New("instance service has no startup command")
}
