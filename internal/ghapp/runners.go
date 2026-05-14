// internal/ghapp/runners.go
package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type RunnerRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *Client) ListRepoRunners(ctx context.Context, repo string) ([]RunnerRef, error) {
	instID, err := c.ResolveInstallation(ctx, repo)
	if err != nil {
		return nil, err
	}
	tok, err := c.FetchInstallationToken(ctx, instID)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/repos/%s/actions/runners?per_page=100", strings.TrimRight(c.APIBase, "/"), repo)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list runners: %s: %s", resp.Status, b)
	}
	var out struct {
		Runners []RunnerRef `json:"runners"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Runners, nil
}

// ListRepoRunnersETag is the ETag-aware variant of ListRepoRunners. Pass an
// empty etag for the first call; pass the returned etag on subsequent calls.
// On 304 (no change) the returned slice is nil and the returned etag echoes
// the input.
func (c *Client) ListRepoRunnersETag(ctx context.Context, repo, etag string) ([]RunnerRef, string, error) {
	instID, err := c.ResolveInstallation(ctx, repo)
	if err != nil {
		return nil, "", err
	}
	tok, err := c.FetchInstallationToken(ctx, instID)
	if err != nil {
		return nil, "", err
	}
	url := fmt.Sprintf("%s/repos/%s/actions/runners?per_page=100", strings.TrimRight(c.APIBase, "/"), repo)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	newETag := resp.Header.Get("ETag")
	if resp.StatusCode == http.StatusNotModified {
		return nil, newETag, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("list runners: %s: %s", resp.Status, b)
	}
	var out struct {
		Runners []RunnerRef `json:"runners"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	return out.Runners, newETag, nil
}

func (c *Client) DeleteRepoRunner(ctx context.Context, repo string, id int64) error {
	instID, err := c.ResolveInstallation(ctx, repo)
	if err != nil {
		return err
	}
	tok, err := c.FetchInstallationToken(ctx, instID)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/repos/%s/actions/runners/%d", strings.TrimRight(c.APIBase, "/"), repo, id)
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete runner %d: %s: %s", id, resp.Status, b)
	}
	return nil
}
