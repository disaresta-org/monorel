package bitbucket

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrPlanGate is returned when Bitbucket rejects an API or git
// operation with HTTP 402 because the workspace plan isn't
// configured. The user must accept a plan at
// bitbucket.org/<workspace>/workspace/settings/plans before monorel
// can push commits or call certain APIs.
//
// 402 is most commonly observed on git push (which monorel doesn't
// invoke directly; the orchestrator's git layer surfaces it). The
// REST client surfaces it for completeness.
var ErrPlanGate = errors.New("bitbucket: workspace plan not configured (visit bitbucket.org/<workspace>/workspace/settings/plans)")

// ErrRateLimited is returned when Bitbucket responds with HTTP 429.
// Callers may retry after a short delay; the response's Retry-After
// header is not yet parsed by this client.
var ErrRateLimited = errors.New("bitbucket: rate limited (HTTP 429); retry after a short delay")

// errorResponse is Bitbucket's error envelope shape.
type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
	} `json:"error"`
}

// mapStatusError converts an HTTP error response into a wrapped Go
// error with a user-actionable message. Includes workspace/repo
// context for 404s.
func mapStatusError(status int, body []byte, workspace, repo string) error {
	msg := decodeErrorMessage(body)
	switch status {
	case 401:
		return fmt.Errorf("bitbucket: auth failed (check BITBUCKET_EMAIL + BITBUCKET_TOKEN); verify the token has Bitbucket scopes: %s", msg)
	case 402:
		return ErrPlanGate
	case 403:
		return fmt.Errorf("bitbucket: forbidden; the token is missing a scope (required: read/write repository, read/write pullrequest): %s", msg)
	case 404:
		return fmt.Errorf("bitbucket: not found (workspace=%q repo=%q); verify the slugs and that you have access: %s", workspace, repo, msg)
	case 429:
		return ErrRateLimited
	case 400:
		return fmt.Errorf("bitbucket: bad input: %s", msg)
	}
	if status >= 500 {
		return fmt.Errorf("bitbucket: server error %d: %s", status, msg)
	}
	return fmt.Errorf("bitbucket: unexpected status %d: %s", status, msg)
}

// decodeErrorMessage tries to extract a human-readable message from
// Bitbucket's error envelope. Falls back to the raw body when the
// envelope doesn't parse.
func decodeErrorMessage(body []byte) string {
	var er errorResponse
	if err := json.Unmarshal(body, &er); err == nil && er.Error.Message != "" {
		if er.Error.Detail != "" {
			return er.Error.Message + " (" + er.Error.Detail + ")"
		}
		return er.Error.Message
	}
	if len(body) == 0 {
		return "(empty response body)"
	}
	if len(body) > 200 {
		return string(body[:200]) + "..."
	}
	return string(body)
}
