package talos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
)

// expandPath resolves ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Client wraps the Talos API client with pvt-specific operations.
type Client struct {
	api      *talosclient.Client
	endpoint string
}

// NewClient creates a Talos API client from a talosconfig file and context.
func NewClient(ctx context.Context, configPath string, contextName string, endpoints []string) (*Client, error) {
	var opts []talosclient.OptionFunc

	if configPath != "" {
		opts = append(opts, talosclient.WithConfigFromFile(expandPath(configPath)))
	} else {
		opts = append(opts, talosclient.WithDefaultConfig())
	}

	if contextName != "" {
		opts = append(opts, talosclient.WithContextName(contextName))
	}

	if len(endpoints) > 0 {
		opts = append(opts, talosclient.WithEndpoints(endpoints...))
	}

	c, err := talosclient.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating talos client: %w", err)
	}

	ep := ""
	if eps := c.GetEndpoints(); len(eps) > 0 {
		ep = eps[0]
	}

	return &Client{
		api:      c,
		endpoint: ep,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.api.Close()
}

// API returns the underlying Talos client for direct access when needed.
func (c *Client) API() *talosclient.Client {
	return c.api
}
