package bitbucket

import (
	"context"
	"fmt"
)

// resolveUsername returns the Bitbucket username associated with the
// configured email + token. Probes /2.0/user on first call and caches
// the result; concurrent first calls share the probe via sync.Once.
func (c *client) resolveUsername(ctx context.Context) (string, error) {
	c.identityOnce.Do(func() {
		resp, err := c.do(ctx, "GET", "/user", nil, nil)
		if err != nil {
			c.identityErr = fmt.Errorf("bitbucket: probe /2.0/user: %w", err)
			return
		}
		var body struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			UUID        string `json:"uuid"`
		}
		if err := decodeJSON(resp, &body); err != nil {
			c.identityErr = err
			return
		}
		if body.Username == "" {
			c.identityErr = fmt.Errorf("bitbucket: /2.0/user returned empty username")
			return
		}
		c.username = body.Username
	})
	if c.identityErr != nil {
		return "", c.identityErr
	}
	return c.username, nil
}
