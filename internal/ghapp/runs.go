// internal/ghapp/runs.go
package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WorkflowRun struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`       // queued | in_progress | completed
	Conclusion   string    `json:"conclusion"`   // success | failure | cancelled | ""
	HeadBranch   string    `json:"head_branch"`
	RunStartedAt time.Time `json:"run_started_at"`
}

// ListWorkflowRuns returns workflow runs filtered by status ("queued",
// "in_progress", or "" for all recent). Passing a non-empty etag sends
// If-None-Match; on 304 the returned slice is nil and the returned etag echoes
// the input (callers should treat that as "no change").
func (c *Client) ListWorkflowRuns(ctx context.Context, repo, status, etag string) ([]WorkflowRun, string, error) {
	instID, err := c.ResolveInstallation(ctx, repo)
	if err != nil {
		return nil, "", err
	}
	tok, err := c.FetchInstallationToken(ctx, instID)
	if err != nil {
		return nil, "", err
	}
	q := url.Values{}
	q.Set("per_page", "10")
	if status != "" {
		q.Set("status", status)
	}
	u := fmt.Sprintf("%s/repos/%s/actions/runs?%s", strings.TrimRight(c.APIBase, "/"), repo, q.Encode())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
		return nil, "", fmt.Errorf("list runs: %s: %s", resp.Status, b)
	}
	var out struct {
		Runs []WorkflowRun `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", err
	}
	return out.Runs, newETag, nil
}
