package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"pbmanager/backend/internal/platform"
)

func (s *server) databaseRequest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input platform.DatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || !platform.ValidateID(input.InstanceID) {
		http.Error(w, "invalid database request", 400)
		return
	}
	if input.Method != "GET" && input.Method != "POST" && input.Method != "PATCH" && input.Method != "DELETE" {
		http.Error(w, "unsupported database method", 400)
		return
	}
	parsed, err := url.ParseRequestURI(input.Path)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/api/") || path.Clean(parsed.Path) != strings.TrimSuffix(parsed.Path, "/") || strings.Contains(parsed.Path, "\\") {
		http.Error(w, "use a relative PocketBase API path beginning with /api/", 400)
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	m, err := s.load(input.InstanceID)
	if err != nil {
		http.Error(w, "unknown managed instance", 404)
		return
	}
	if _, err = command("mountpoint", "-q", s.path(input.InstanceID)); err != nil {
		http.Error(w, "instance storage is not mounted", 409)
		return
	}
	if _, err = command("systemctl", "is-active", "--quiet", s.unit(input.InstanceID)); err != nil {
		http.Error(w, "start the instance before accessing its database", 409)
		return
	}
	token, err := s.databaseToken(m.Instance)
	if err != nil {
		http.Error(w, "could not establish instance administrator access: "+err.Error(), 500)
		return
	}
	endpoint := url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(m.Instance.Port), Path: parsed.Path, RawQuery: parsed.RawQuery}
	request, err := http.NewRequestWithContext(r.Context(), input.Method, endpoint.String(), bytes.NewReader(input.Body))
	if err != nil {
		http.Error(w, "invalid database request", 400)
		return
	}
	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		http.Error(w, "instance API is unavailable", 502)
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		http.Error(w, "instance response exceeded 8 MiB; use pagination", 502)
		return
	}
	if len(body) == 0 {
		body = []byte("null")
	} else if !json.Valid(body) {
		body, _ = json.Marshal(string(body))
	}
	writeJSON(w, platform.DatabaseResponse{Status: response.StatusCode, Body: body})
}

// A dedicated service identity avoids changing or disclosing customer passwords.
func (s *server) databaseToken(instance platform.Instance) (string, error) {
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: filepath.Join(s.path(instance.ID), "pb_data")})
	if err := app.Bootstrap(); err != nil {
		return "", err
	}
	defer app.ResetBootstrapState()
	email := "pbmanager-mcp@" + instance.Hostname
	record, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, email)
	if err != nil {
		collection, findErr := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if findErr != nil {
			return "", findErr
		}
		record = core.NewRecord(collection)
		record.SetEmail(email)
		record.SetPassword(platform.RandomString(48))
		if err = app.Save(record); err != nil {
			return "", err
		}
	}
	if record == nil {
		return "", errors.New("service identity is unavailable")
	}
	return record.NewStaticAuthToken(5 * time.Minute)
}
