//go:build !linux

package agent

import "errors"

func Run() error { return errors.New("systemd provisioning is supported on Linux only") }
