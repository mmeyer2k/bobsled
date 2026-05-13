// internal/ghapp/jit.go
package ghapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type JITRequest struct {
	Name   string
	Labels []string
}

type JITResponse struct {
	Runner struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"runner"`
	EncodedJITConfig string `json:"encoded_jit_config"`
}

func (c *Client) GenerateJITConfig(ctx context.Context, repo string, req JITRequest) (*JITResponse, error) {
	instID, err := c.ResolveInstallation(ctx, repo)
	if err != nil {
		return nil, err
	}
	tok, err := c.FetchInstallationToken(ctx, instID)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{
		"name":            req.Name,
		"runner_group_id": 1,
		"labels":          req.Labels,
		"work_folder":     "_work",
	})
	url := fmt.Sprintf("%s/repos/%s/actions/runners/generate-jitconfig", strings.TrimRight(c.APIBase, "/"), repo)
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "token "+tok)
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jitconfig: %s: %s", resp.Status, b)
	}
	var out JITResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
