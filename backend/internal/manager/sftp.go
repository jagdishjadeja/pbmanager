package manager

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
)

func (m *Manager) createSFTPUser(e *core.RequestEvent) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, 4096)
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("Invalid file account request.", nil)
	}
	if err := platform.ValidateSFTPUsername(body.Username); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	if err := platform.ValidateSFTPPassword(body.Password); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.siteInstance(e)
	if err != nil {
		return err
	}
	items, err := instances(m.app)
	if err != nil {
		return e.InternalServerError("Could not check file accounts.", err)
	}
	for _, item := range items {
		for _, account := range item.SFTPUsers {
			if account.Username == body.Username && item.ID != i.ID {
				return e.Error(409, "Username is assigned to another instance.", nil)
			}
		}
	}
	operation, err := m.enqueue(i, "sftp-create", platform.AgentRequest{Instance: i, Action: "sftp-create", Username: body.Username, Password: body.Password})
	if err != nil {
		return e.Error(409, err.Error(), nil)
	}
	return e.JSON(202, operation)
}
func (m *Manager) removeSFTPUser(e *core.RequestEvent) error {
	username := e.Request.PathValue("username")
	if err := platform.ValidateSFTPUsername(username); err != nil {
		return e.BadRequestError(err.Error(), nil)
	}
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.siteInstance(e)
	if err != nil {
		return err
	}
	operation, err := m.enqueue(i, "sftp-remove", platform.AgentRequest{Instance: i, Action: "sftp-remove", Username: username})
	if err != nil {
		return e.Error(409, err.Error(), nil)
	}
	return e.JSON(202, operation)
}
