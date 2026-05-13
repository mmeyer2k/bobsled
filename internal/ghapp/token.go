// internal/ghapp/token.go
package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	APIBase string
	AppID   int64
	KeyPath string
	HTTP    *http.Client
	Now     func() time.Time
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) ResolveInstallation(ctx context.Context, repo string) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/installation", strings.TrimRight(c.APIBase, "/"), repo)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err := c.signAsApp(req); err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("resolve installation: %s: %s", resp.Status, b)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

func (c *Client) FetchInstallationToken(ctx context.Context, installationID int64) (string, error) {
	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", strings.TrimRight(c.APIBase, "/"), installationID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err := c.signAsApp(req); err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("installation token: %s: %s", resp.Status, b)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Token, nil
}

func (c *Client) signAsApp(req *http.Request) error {
	tok, err := SignAppJWT(c.KeyPath, c.AppID, c.Now)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}
