package manager

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"pbmanager/backend/internal/platform"
)

func (f *fakeAgent) PublishSite(id, filename, format string, reader io.Reader) (platform.StaticSite, error) {
	data, err := io.ReadAll(reader)
	site := platform.StaticSite{Filename: filename, Files: 1, Bytes: int64(len(data)), PublishedAt: platform.Now()}
	instance := f.items[id]
	instance.StaticSite = &site
	f.items[id] = instance
	return site, err
}
func (f *fakeAgent) RemoveSite(id string) error {
	instance := f.items[id]
	instance.StaticSite = nil
	f.items[id] = instance
	return nil
}

func TestSiteUploadAuthorizationPersistenceAndRemoval(t *testing.T) {
	m, handler, cookie := setup(t)
	created := perform(handler, "POST", "/api/manager/instances", `{"name":"site-demo"}`, cookie)
	if created.Code != 202 || !m.work() {
		t.Fatal("could not create test instance")
	}
	items, _ := instances(m.app)
	id := items[0].ID
	path := "/api/manager/instances/" + id + "/site"
	upload := func(cookie *http.Cookie, filename, origin string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, _ := writer.CreateFormFile("site", filename)
		_, _ = file.Write([]byte("<h1>My website</h1>"))
		_ = writer.Close()
		request := httptest.NewRequest("POST", "http://manager.example.com"+path, &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		if cookie != nil {
			request.AddCookie(cookie)
		}
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := upload(nil, "site.html", ""); response.Code != 401 {
		t.Fatalf("unauthenticated upload: %d", response.Code)
	}
	if response := upload(cookie, "site.html", "https://evil.example"); response.Code != 403 {
		t.Fatalf("foreign origin: %d", response.Code)
	}
	if response := upload(cookie, "hook.js", ""); response.Code != 400 {
		t.Fatalf("invalid extension: %d", response.Code)
	}
	if response := upload(cookie, "site.html", ""); response.Code != 200 {
		t.Fatalf("upload: %d %s", response.Code, response.Body.String())
	}
	m.refresh()
	instance, _ := m.find(id)
	if instance.StaticSite == nil || instance.StaticSite.Filename != "site.html" || instance.DatabaseBytes != 4096 {
		t.Fatalf("site metadata or database changed: %+v", instance)
	}
	response := perform(handler, "DELETE", path, "", cookie)
	if response.Code != 204 {
		t.Fatalf("remove: %d %s", response.Code, response.Body.String())
	}
	m.refresh()
	instance, _ = m.find(id)
	if instance.StaticSite != nil || instance.Status != "running" || instance.DatabaseBytes != 4096 {
		t.Fatal("site removal affected PocketBase")
	}
}
