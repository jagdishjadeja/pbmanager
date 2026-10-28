package manager

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
	"pbmanager/backend/internal/staticfiles"
)

func (m *Manager) siteInstance(e *core.RequestEvent) (platform.Instance, error) {
	i, err := m.find(e.Request.PathValue("id"))
	if err != nil {
		return i, e.NotFoundError("Instance not found.", nil)
	}
	jobs, err := m.app.FindRecordsByFilter("manager_operations", "instance={:id} && (state='queued' || state='running')", "", 1, 0, dbx.Params{"id": i.ID})
	if err != nil {
		return i, e.InternalServerError("Could not check instance operations.", err)
	}
	if len(jobs) > 0 || i.Status == "provisioning" {
		return i, e.Error(409, "Wait for the current instance operation to finish.", nil)
	}
	return i, nil
}

func (m *Manager) uploadSite(e *core.RequestEvent) error {
	// Parse outside the mutation lock, so a slow browser upload does not block jobs.
	e.Request.Body = http.MaxBytesReader(e.Response, e.Request.Body, staticfiles.MaxUploadBytes+(1<<20))
	if err := e.Request.ParseMultipartForm(1 << 20); err != nil {
		return e.BadRequestError("Upload one ZIP archive or HTML file, up to 50 MiB.", nil)
	}
	defer e.Request.MultipartForm.RemoveAll()
	files := e.Request.MultipartForm.File["site"]
	if len(files) != 1 || len(e.Request.MultipartForm.File) != 1 {
		return e.BadRequestError("Choose one ZIP archive or HTML file.", nil)
	}
	file := files[0]
	if file.Size < 1 || file.Size > staticfiles.MaxUploadBytes {
		return e.Error(413, "The site upload must be between 1 byte and 50 MiB.", nil)
	}
	format := ""
	switch strings.ToLower(filepath.Ext(file.Filename)) {
	case ".zip":
		format = "zip"
	case ".html", ".htm":
		format = "html"
	default:
		return e.BadRequestError("Choose a .zip or .html file.", nil)
	}
	source, err := file.Open()
	if err != nil {
		return e.InternalServerError("Could not read the uploaded file.", err)
	}
	defer source.Close()
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.siteInstance(e)
	if err != nil {
		return err
	}
	site, err := m.agent.PublishSite(i.ID, file.Filename, format, io.LimitReader(source, staticfiles.MaxUploadBytes+1))
	if err != nil {
		return e.Error(422, "Could not publish static site: "+err.Error(), nil)
	}
	i.StaticSite = &site
	i.UpdatedAt = platform.Now()
	if err = saveInstance(m.app, i); err != nil {
		return e.InternalServerError("The site was published but its dashboard status could not be saved. Refresh to retry.", err)
	}
	m.event(i, "Static site published: "+site.Filename, "success")
	return e.JSON(200, site)
}

func (m *Manager) removeSite(e *core.RequestEvent) error {
	m.mutation.Lock()
	defer m.mutation.Unlock()
	i, err := m.siteInstance(e)
	if err != nil {
		return err
	}
	if err = m.agent.RemoveSite(i.ID); err != nil {
		return e.Error(502, "Could not remove static site: "+err.Error(), nil)
	}
	i.StaticSite = nil
	i.UpdatedAt = platform.Now()
	if err = saveInstance(m.app, i); err != nil {
		return e.InternalServerError("Could not save site removal.", err)
	}
	m.event(i, "Static site removed", "info")
	return e.NoContent(204)
}
