package bitbucket

import (
	"context"
	"fmt"
)

// GetDefaultBranch returns the repo's default branch by reading
// `mainbranch.name` from the repository resource.
func (c *client) GetDefaultBranch(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, "GET", c.repoBase(), nil, nil)
	if err != nil {
		return "", err
	}
	var body struct {
		Mainbranch struct {
			Name string `json:"name"`
		} `json:"mainbranch"`
	}
	if err := decodeJSON(resp, &body); err != nil {
		return "", err
	}
	if body.Mainbranch.Name == "" {
		return "", fmt.Errorf("bitbucket: %s/%s has no default branch", c.workspace, c.repo)
	}
	return body.Mainbranch.Name, nil
}
