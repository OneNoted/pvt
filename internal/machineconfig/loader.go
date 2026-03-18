package machineconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OneNoted/pvt/internal/config"
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

// ResolvePath returns the expected file path for a node's machine config
// without reading it. Useful for dry-run display and pre-flight validation.
func ResolvePath(source config.ConfigSource, clusterName, nodeName string) string {
	base := expandPath(source.Path)

	switch source.Type {
	case "talhelper":
		return filepath.Join(base, "clusterconfig", clusterName+"-"+nodeName+".yaml")
	default: // "directory"
		return filepath.Join(base, nodeName+".yaml")
	}
}

// LoadMachineConfig reads a Talos machine config YAML file for the given node.
// Returns the raw bytes suitable for passing to the Talos ApplyConfiguration API.
func LoadMachineConfig(source config.ConfigSource, clusterName, nodeName string) ([]byte, error) {
	path := ResolvePath(source, clusterName, nodeName)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading machine config for %q: %w (expected at %s)", nodeName, err, path)
	}

	return data, nil
}
