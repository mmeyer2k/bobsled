// internal/ghapp/installations.go
package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

type installationRef struct {
	ID int64 `json:"id"`
}

// ListAccessibleRepos returns the set of "owner/name" full names that this App
// can manage, across every installation it's been added to. Deduplicated and
// sorted alphabetically.
func (c *Client) ListAccessibleRepos(ctx context.Context) ([]string, error) {
	installs, err := c.listInstallations(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, inst := range installs {
		repos, err := c.listRepoFullNames(ctx, inst.ID)
		if err != nil {
			return nil, fmt.Errorf("install %d: %w", inst.ID, err)
		}
		for _, r := range repos {
			seen[r] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out, nil
}

func (c *Client) listInstallations(ctx context.Context) ([]installationRef, error) {
	url := fmt.Sprintf("%s/app/installations", strings.TrimRight(c.APIBase, "/"))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err := c.signAsApp(req); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list installations: %s: %s", resp.Status, b)
	}
	var out []installationRef
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// nextLink finds the rel="next" URL in a GitHub Link header, or "" if absent.
var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextLink(h string) string {
	if h == "" {
		return ""
	}
	m := linkRE.FindStringSubmatch(h)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func (c *Client) listRepoFullNames(ctx context.Context, installationID int64) ([]string, error) {
	tok, err := c.FetchInstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/installation/repositories?per_page=100", strings.TrimRight(c.APIBase, "/"))
	var out []string
	for url != "" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("Authorization", "token "+tok)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, err
		}
		var body struct {
			Repositories []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("list repos: %s: %s", resp.Status, b)
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			return nil, err
		}
		for _, r := range body.Repositories {
			out = append(out, r.FullName)
		}
		url = nextLink(resp.Header.Get("Link"))
		resp.Body.Close()
	}
	return out, nil
}
