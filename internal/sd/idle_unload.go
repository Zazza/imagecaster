package sd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

func (c *Client) UnloadIdleModel(ctx context.Context) error {
	if err := c.do(ctx, http.MethodPost, "/unload", map[string]any{}, nil); err != nil {
		return fmt.Errorf("unload idle SD model: %w", err)
	}
	return nil
}

func (c *Client) IdleModelServerURL() string {
	base := strings.TrimRight(c.base(), "/")
	if !strings.HasSuffix(base, "/api/sd") {
		return ""
	}
	return strings.TrimSuffix(base, "/api/sd")
}
