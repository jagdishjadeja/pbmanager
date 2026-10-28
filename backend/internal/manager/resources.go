package manager

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
)

func (m *Manager) updateResources(e *core.RequestEvent) error {
	var body struct {
		MemoryLimitMiB int `json:"memoryLimitMiB"`
	}
	e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, 4096)
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("Enter a RAM limit in MiB.", nil)
	}
	limit, err := platform.MemoryLimit(body.MemoryLimitMiB)
	if err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.siteInstance(e)
	if err != nil {
		return err
	}
	i.MemoryLimitBytes = limit
	operation, err := m.enqueue(i, "resources", platform.AgentRequest{Instance: i, Action: "resources"})
	if err != nil {
		return e.Error(409, err.Error(), nil)
	}
	return e.JSON(202, operation)
}
