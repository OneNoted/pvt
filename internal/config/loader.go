package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a pvt config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return Parse(data)
}

// Parse parses raw YAML bytes into a Config.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Discover attempts to find the pvt config file in standard locations.
// Search order: $PVT_CONFIG, ./pvt.yaml, ~/.config/pvt/config.yaml
func Discover() (string, error) {
	if env := os.Getenv("PVT_CONFIG"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env, nil
		}
	}

	if _, err := os.Stat("pvt.yaml"); err == nil {
		abs, _ := filepath.Abs("pvt.yaml")
		return abs, nil
	}

	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".config", "pvt", "config.yaml")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("no pvt config file found (searched: $PVT_CONFIG, ./pvt.yaml, ~/.config/pvt/config.yaml)")
}

// validate performs structural validation on a parsed config.
func validate(cfg *Config) error {
	if cfg.Version == "" {
		return fmt.Errorf("config: version is required")
	}

	if cfg.Version != "1" {
		return fmt.Errorf("config: unsupported version %q (supported: \"1\")", cfg.Version)
	}

	if len(cfg.Proxmox.Clusters) == 0 {
		return fmt.Errorf("config: at least one proxmox cluster must be defined")
	}

	for i, pc := range cfg.Proxmox.Clusters {
		if pc.Name == "" {
			return fmt.Errorf("config: proxmox.clusters[%d].name is required", i)
		}
		if pc.Endpoint == "" {
			return fmt.Errorf("config: proxmox.clusters[%d].endpoint is required", i)
		}
	}

	if len(cfg.Clusters) == 0 {
		return fmt.Errorf("config: at least one cluster must be defined")
	}

	for i, c := range cfg.Clusters {
		if c.Name == "" {
			return fmt.Errorf("config: clusters[%d].name is required", i)
		}
		if c.ProxmoxCluster == "" {
			return fmt.Errorf("config: clusters[%d].proxmox_cluster is required", i)
		}
		if c.Endpoint == "" {
			return fmt.Errorf("config: clusters[%d].endpoint is required", i)
		}
		if len(c.Nodes) == 0 {
			return fmt.Errorf("config: clusters[%d].nodes must not be empty", i)
		}
		for j, n := range c.Nodes {
			if n.Name == "" {
				return fmt.Errorf("config: clusters[%d].nodes[%d].name is required", i, j)
			}
			if n.Role != "controlplane" && n.Role != "worker" {
				return fmt.Errorf("config: clusters[%d].nodes[%d].role must be \"controlplane\" or \"worker\", got %q", i, j, n.Role)
			}
			if n.ProxmoxVMID == 0 {
				return fmt.Errorf("config: clusters[%d].nodes[%d].proxmox_vmid is required", i, j)
			}
			if n.ProxmoxNode == "" {
				return fmt.Errorf("config: clusters[%d].nodes[%d].proxmox_node is required", i, j)
			}
			if n.IP == "" {
				return fmt.Errorf("config: clusters[%d].nodes[%d].ip is required", i, j)
			}
		}

		// Verify proxmox_cluster reference exists
		found := false
		for _, pc := range cfg.Proxmox.Clusters {
			if pc.Name == c.ProxmoxCluster {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("config: clusters[%d].proxmox_cluster %q does not match any defined proxmox cluster", i, c.ProxmoxCluster)
		}
	}

	return nil
}
