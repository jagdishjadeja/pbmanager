package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"pbmanager/backend/internal/platform"
)

type Client struct{ HTTP *http.Client }

func NewClient(socket string) *Client {
	return &Client{HTTP: &http.Client{Timeout: 3 * time.Minute, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}}
}

func (c *Client) call(method, path string, body any, result any) error {
	var buffer bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buffer).Encode(body); err != nil {
			return err
		}
	}
	request, err := http.NewRequest(method, "http://agent"+path, &buffer)
	if err != nil {
		return err
	}
	if method == "GET" && len(path) >= 10 && path[:10] == "/progress/" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		request = request.WithContext(ctx)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("provisioning agent unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("agent: %s", string(data))
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(result)
	}
	return nil
}

func (c *Client) Execute(request platform.AgentRequest) error {
	return c.call("POST", "/execute", request, nil)
}
func (c *Client) Snapshot() (platform.AgentSnapshot, error) {
	var result platform.AgentSnapshot
	err := c.call("GET", "/snapshot", nil, &result)
	return result, err
}
func (c *Client) Logs(id string) ([]string, error) {
	var result []string
	err := c.call("GET", "/logs/"+id, nil, &result)
	return result, err
}
func (c *Client) Progress(id string) (platform.Progress, error) {
	var result platform.Progress
	err := c.call("GET", "/progress/"+id, nil, &result)
	return result, err
}

func (c *Client) PublishSite(id, filename, format string, source io.Reader) (platform.StaticSite, error) {
	var site platform.StaticSite
	request, err := http.NewRequest("POST", "http://agent/sites/"+id, source)
	if err != nil {
		return site, err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Site-Format", format)
	request.Header.Set("X-Site-Filename", filename)
	response, err := c.HTTP.Do(request)
	if err != nil {
		return site, fmt.Errorf("static site agent unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return site, fmt.Errorf("%s", bytes.TrimSpace(message))
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&site)
	return site, err
}

func (c *Client) RemoveSite(id string) error { return c.call("DELETE", "/sites/"+id, nil, nil) }

func (c *Client) Database(request platform.DatabaseRequest) (platform.DatabaseResponse, error) {
	var response platform.DatabaseResponse
	err := c.call("POST", "/database", request, &response)
	return response, err
}
