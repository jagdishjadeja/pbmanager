package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"pbmanager/backend/internal/platform"
)

const sftpGroup = "pbmanager-sftp"

func sftpRoot(username string) string { return filepath.Join("/var/lib/pbmanager/sftp", username) }
func sftpUnit(username string) string { return "var-lib-pbmanager-sftp-" + username + ".mount" }

func (s *server) createSFTP(m *manifest, username, password string) error {
	if err := platform.ValidateSFTPUsername(username); err != nil {
		return err
	}
	if err := platform.ValidateSFTPPassword(password); err != nil {
		return err
	}
	existing := slices.ContainsFunc(m.Instance.SFTPUsers, func(u platform.SFTPUser) bool { return u.Username == username })
	if existing {
		account, err := user.Lookup(username)
		if err == nil {
			uid, _ := strconv.Atoi(account.Uid)
			for _, entry := range m.Instance.SFTPUsers {
				if entry.Username == username && entry.UID != 0 && entry.UID != uid {
					return errors.New("the Linux account identity has changed")
				}
			}
		}
	}
	if _, err := user.Lookup(username); err == nil && !existing {
		return errors.New("this Linux username is already in use")
	}
	items, err := s.all()
	if err != nil {
		return err
	}
	for _, other := range items {
		if other.Instance.ID != m.Instance.ID {
			for _, account := range other.Instance.SFTPUsers {
				if account.Username == username {
					return errors.New("username belongs to another instance")
				}
			}
		}
	}
	if _, err = command("mountpoint", "-q", s.path(m.Instance.ID)); err != nil {
		return errors.New("instance storage is not mounted")
	}
	if err = s.ensureSFTPLayout(m.Instance.ID); err != nil {
		return err
	}
	if err = s.configurePublicDir(m.Instance); err != nil {
		return err
	}
	if _, err = user.LookupGroup(sftpGroup); err != nil {
		if _, err = command("groupadd", "--system", sftpGroup); err != nil {
			return err
		}
	}
	created := false
	if _, err = user.Lookup(username); err != nil {
		if _, err = command("useradd", "--no-create-home", "--home-dir", "/current", "--shell", "/usr/sbin/nologin", "--gid", "pbm-"+m.Instance.ID, "--groups", sftpGroup, username); err != nil {
			return err
		}
		created = true
	}
	ok := false
	defer func() {
		if created && !ok {
			_, _ = command("systemctl", "disable", "--now", sftpUnit(username))
			_ = os.Remove(filepath.Join("/etc/systemd/system", sftpUnit(username)))
			_ = os.Remove(sftpRoot(username))
			_, _ = command("userdel", username)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "chpasswd")
	cmd.Stdin = strings.NewReader(username + ":" + password + "\n")
	if err = cmd.Run(); err != nil {
		return errors.New("could not set the file account password")
	}
	for _, path := range []string{"/var/lib/pbmanager/sftp", sftpRoot(username)} {
		if err = os.MkdirAll(path, 0755); err != nil {
			return err
		}
		if err = os.Chmod(path, 0755); err != nil {
			return err
		}
		if err = os.Chown(path, 0, 0); err != nil {
			return err
		}
	}
	mount := fmt.Sprintf("[Unit]\nDescription=Scoped website files for %s\nAfter=%s\nRequires=%s\nBefore=ssh.service\n[Mount]\nWhat=%s\nWhere=%s\nType=none\nOptions=bind,nodev,nosuid,noexec\n[Install]\nWantedBy=multi-user.target\n", username, s.mountUnit(m.Instance.ID), s.mountUnit(m.Instance.ID), filepath.Join(s.path(m.Instance.ID), "site"), sftpRoot(username))
	if err = atomicWrite(filepath.Join("/etc/systemd/system", sftpUnit(username)), []byte(mount), 0644); err != nil {
		return err
	}
	if _, err = command("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if _, err = command("systemctl", "enable", "--now", sftpUnit(username)); err != nil {
		return err
	}
	if err = configureSFTPDaemon(); err != nil {
		return err
	}
	if !existing {
		account, err := user.Lookup(username)
		if err != nil {
			return err
		}
		uid, _ := strconv.Atoi(account.Uid)
		m.Instance.SFTPUsers = append(m.Instance.SFTPUsers, platform.SFTPUser{Username: username, UID: uid, CreatedAt: platform.Now()})
	}
	if err = s.save(m); err != nil {
		return err
	}
	ok = true
	return nil
}

func configureSFTPDaemon() error {
	path := "/etc/ssh/sshd_config.d/70-pbmanager-sftp.conf"
	config := []byte("# Managed PocketBase website file accounts.\nMatch Group pbmanager-sftp\n    ChrootDirectory /var/lib/pbmanager/sftp/%u\n    ForceCommand internal-sftp -d /current -u 0022 -P symlink,hardlink\n    PasswordAuthentication yes\n    KbdInteractiveAuthentication no\n    DisableForwarding yes\n    PermitTTY no\n    X11Forwarding no\n    PermitTunnel no\nMatch all\n")
	previous, readErr := os.ReadFile(path)
	if err := atomicWrite(path, config, 0644); err != nil {
		return err
	}
	if _, err := command("/usr/sbin/sshd", "-t"); err != nil {
		if readErr == nil {
			_ = atomicWrite(path, previous, 0644)
		} else {
			_ = os.Remove(path)
		}
		return err
	}
	if _, err := command("systemctl", "reload", "ssh"); err != nil {
		return err
	}
	return nil
}

func (s *server) removeSFTP(m *manifest, username string) error {
	index := slices.IndexFunc(m.Instance.SFTPUsers, func(u platform.SFTPUser) bool { return u.Username == username })
	if index < 0 {
		return errors.New("file account is not assigned to this instance")
	}
	if err := platform.ValidateSFTPUsername(username); err != nil {
		return err
	}
	if account, err := user.Lookup(username); err == nil {
		uid, _ := strconv.Atoi(account.Uid)
		if m.Instance.SFTPUsers[index].UID != 0 && m.Instance.SFTPUsers[index].UID != uid {
			return errors.New("the Linux account identity has changed")
		}
	}
	// These users have no general-purpose processes; disconnect active SFTP sessions.
	_, _ = command("pkill", "-KILL", "-u", username)
	if _, err := command("systemctl", "disable", "--now", sftpUnit(username)); err != nil {
		return err
	}
	if _, err := command("mountpoint", "-q", sftpRoot(username)); err == nil {
		return errors.New("file account mount is still active")
	}
	if _, err := user.Lookup(username); err == nil {
		if _, err = command("userdel", username); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join("/etc/systemd/system", sftpUnit(username))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(sftpRoot(username)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, _ = command("systemctl", "daemon-reload")
	m.Instance.SFTPUsers = slices.Delete(m.Instance.SFTPUsers, index, index+1)
	return s.save(m)
}

func (s *server) ensureSFTPLayout(id string) error {
	if _, err := command("mountpoint", "-q", s.path(id)); err != nil {
		return errors.New("instance storage is not mounted")
	}
	old, err := s.publicTarget(id)
	if err != nil {
		return err
	}
	for _, path := range []string{filepath.Join(s.path(id), "site"), filepath.Join(s.path(id), "site", ".releases")} {
		if err = os.MkdirAll(path, 0755); err != nil {
			return err
		}
		if err = os.Chmod(path, 0755); err != nil {
			return err
		}
		if err = os.Chown(path, 0, 0); err != nil {
			return err
		}
	}
	if !strings.HasPrefix(old, "site/") {
		if old == "" {
			stage, err := os.MkdirTemp(filepath.Join(s.path(id), "site", ".releases"), ".site-release-")
			if err != nil {
				return err
			}
			old = filepath.Join("site", ".releases", filepath.Base(stage))
		} else {
			next := filepath.Join("site", ".releases", filepath.Base(old))
			if err = os.Rename(filepath.Join(s.path(id), old), filepath.Join(s.path(id), next)); err != nil {
				return err
			}
			old = next
		}
		if err = s.switchPublic(id, old); err != nil {
			return err
		}
	}
	return s.sitePermissions(filepath.Join(s.path(id), old), id)
}

func (s *server) sitePermissions(root, id string) error {
	_, layoutErr := os.Stat(filepath.Join(s.path(id), "site", ".releases"))
	gid := -1
	if layoutErr == nil {
		group, err := user.LookupGroup("pbm-" + id)
		if err != nil {
			return err
		}
		gid, _ = strconv.Atoi(group.Gid)
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("unexpected symlink in public files")
		}
		mode := os.FileMode(0644)
		if entry.IsDir() {
			mode = 0755
		}
		if gid >= 0 {
			if err := os.Chown(path, 0, gid); err != nil {
				return err
			}
			mode = 0664
			if entry.IsDir() {
				mode = 0775 | os.ModeSetgid
			}
		}
		return os.Chmod(path, mode)
	})
}

func (s *server) measurePublicFiles(instance *platform.Instance) {
	target, err := s.publicTarget(instance.ID)
	if err != nil || target == "" {
		return
	}
	root := filepath.Join(s.path(instance.ID), target)
	if info, err := os.Stat(filepath.Join(root, "index.html")); err != nil || !info.Mode().IsRegular() {
		instance.StaticSite = nil
		return
	}
	site := platform.StaticSite{Filename: "SSH/SFTP files", PublishedAt: platform.Now()}
	if instance.StaticSite != nil {
		site.Filename = instance.StaticSite.Filename
		site.PublishedAt = instance.StaticSite.PublishedAt
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			site.Files++
			site.Bytes += info.Size()
		}
		return nil
	})
	instance.StaticSite = &site
}
