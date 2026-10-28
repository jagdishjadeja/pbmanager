package agent

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"pbmanager/backend/internal/platform"
)

func validateHostMemory(limit int64) error {
	if limit%platform.MiB != 0 {
		return errors.New("RAM must use whole MiB")
	}
	if _, err := platform.MemoryLimit(int(limit / platform.MiB)); err != nil {
		return err
	}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[0] == "MemTotal:" {
			kib, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return err
			}
			maximum := (kib*1024 - 512*platform.MiB) / (64 * platform.MiB) * (64 * platform.MiB)
			if limit > maximum {
				return fmt.Errorf("this server supports a maximum of %d MiB per instance, keeping 512 MiB for the system", maximum/platform.MiB)
			}
			return nil
		}
	}
	return errors.New("could not determine server RAM capacity")
}

func (s *server) updateMemory(m *manifest, limit int64) error {
	if err := validateHostMemory(limit); err != nil {
		return err
	}
	output, err := command("systemctl", "show", s.unit(m.Instance.ID), "--property=MemoryCurrent,MemoryMax")
	if err != nil {
		return err
	}
	previous := m.Instance.MemoryLimitBytes
	if previous == 0 {
		previous = platform.DefaultMemoryMiB * platform.MiB
	}
	for _, line := range strings.Split(output, "\n") {
		key, value, _ := strings.Cut(line, "=")
		number, _ := strconv.ParseInt(value, 10, 64)
		if key == "MemoryCurrent" && number > limit {
			return errors.New("the selected RAM limit is below current usage; choose a larger value or stop the instance first")
		}
		if key == "MemoryMax" && number > 0 {
			previous = number
		}
	}
	_, err = command("systemctl", "set-property", s.unit(m.Instance.ID), fmt.Sprintf("MemoryMax=%d", limit))
	if err != nil {
		return err
	}
	m.Instance.MemoryLimitBytes = limit
	if err = s.save(m); err != nil {
		_, _ = command("systemctl", "set-property", s.unit(m.Instance.ID), fmt.Sprintf("MemoryMax=%d", previous))
		m.Instance.MemoryLimitBytes = previous
	}
	return err
}
