package bitbucket

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// do issues an authenticated HTTP request and decodes the JSON
// response body into out (if non-nil). Returns a wrapped error on
// non-2xx, with status-code-specific messages for the common cases.
//
// path is the API path (e.g. "/repositories/ws/r"); the baseURL is
// prepended. query is appended as URL-encoded parameters when
// non-nil. body, when non-nil, is JSON-encoded and sent as the
// request body with Content-Type: application/json.
func (c *client) do(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, fmt.Errorf("bitbucket: marshal request body: %w", err)
		}
		bodyReader = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, full, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: build request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.email+":"+c.token)))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: %s %s: %w", method, full, err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBytes, _ := io.ReadAll(resp.Body)
		return nil, mapStatusError(resp.StatusCode, respBytes, c.workspace, c.repo)
	}
	return resp, nil
}

// decodeJSON reads resp.Body, JSON-decodes into out, and closes the
// body. Convenience for the common one-shot pattern.
func decodeJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("bitbucket: decode response: %w", err)
	}
	return nil
}

// repoBase returns the API base path for the configured repo, e.g.
// /repositories/<workspace>/<repo>. Includes URL-encoding of the
// workspace and repo slugs (rare but possible if the slug contains
// reserved characters).
func (c *client) repoBase() string {
	return "/repositories/" + url.PathEscape(c.workspace) + "/" + url.PathEscape(c.repo)
}
