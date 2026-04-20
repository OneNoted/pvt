package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/proxmox"
)

// Severity describes how a failed check should affect the command result.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Check is one doctor result.
type Check struct {
	Name     string
	Severity Severity
	OK       bool
	Detail   string
}

// HasErrors reports whether any error-level check failed.
func HasErrors(checks []Check) bool {
	for _, check := range checks {
		if !check.OK && check.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Run executes local and optional connectivity checks.
func Run(ctx context.Context, configPath string) []Check {
	var checks []Check
	if configPath == "" {
		discovered, err := config.Discover()
		if err != nil {
			return []Check{{
				Name:     "config discovery",
				Severity: SeverityError,
				OK:       false,
				Detail:   err.Error(),
			}}
		}
		configPath = discovered
	}

	checks = append(checks, Check{
		Name:     "config discovery",
		Severity: SeverityError,
		OK:       true,
		Detail:   configPath,
	})

	cfg, err := config.Load(configPath)
	if err != nil {
		checks = append(checks, Check{
			Name:     "config parse",
			Severity: SeverityError,
			OK:       false,
			Detail:   err.Error(),
		})
		return checks
	}
	checks = append(checks, Check{Name: "config parse", Severity: SeverityError, OK: true, Detail: "valid"})

	checks = append(checks, binaryCheck("kubectl", "PVT_KUBECTL_BIN", SeverityWarn))
	checks = append(checks, binaryCheck("talosctl", "PVT_TALOSCTL_BIN", SeverityWarn))

	talosPath := expandPath(cfg.Talos.ConfigPath)
	_, err = os.Stat(talosPath)
	checks = append(checks, Check{
		Name:     "talos config",
		Severity: SeverityWarn,
		OK:       err == nil,
		Detail:   detailOrErr(talosPath, err),
	})

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	_, err = os.Stat(kubeconfig)
	checks = append(checks, Check{
		Name:     "kubeconfig",
		Severity: SeverityWarn,
		OK:       err == nil,
		Detail:   detailOrErr(kubeconfig, err),
	})

	for _, cluster := range cfg.Proxmox.Clusters {
		checks = append(checks, Check{
			Name:     "proxmox config " + cluster.Name,
			Severity: SeverityError,
			OK:       cluster.Endpoint != "" && cluster.TokenID != "" && cluster.TokenSecret != "",
			Detail:   proxmoxConfigDetail(cluster),
		})

		client, err := proxmox.NewClient(cluster)
		if err != nil {
			checks = append(checks, Check{
				Name:     "proxmox auth " + cluster.Name,
				Severity: SeverityWarn,
				OK:       false,
				Detail:   err.Error(),
			})
			continue
		}
		err = client.Ping(ctx)
		checks = append(checks, Check{
			Name:     "proxmox auth " + cluster.Name,
			Severity: SeverityWarn,
			OK:       err == nil,
			Detail:   detailOrErr("reachable", err),
		})
	}

	return checks
}

func binaryCheck(name, env string, severity Severity) Check {
	if override := os.Getenv(env); override != "" {
		_, err := os.Stat(override)
		return Check{Name: name, Severity: severity, OK: err == nil, Detail: detailOrErr(override, err)}
	}

	path, err := exec.LookPath(name)
	return Check{Name: name, Severity: severity, OK: err == nil, Detail: detailOrErr(path, err)}
}

func proxmoxConfigDetail(cluster config.ProxmoxCluster) string {
	missing := []string{}
	if cluster.Endpoint == "" {
		missing = append(missing, "endpoint")
	}
	if cluster.TokenID == "" {
		missing = append(missing, "token_id")
	}
	if cluster.TokenSecret == "" {
		missing = append(missing, "token_secret")
	}
	if len(missing) > 0 {
		return "missing " + strings.Join(missing, ", ")
	}
	return cluster.Endpoint
}

func detailOrErr(detail string, err error) string {
	if err != nil {
		return err.Error()
	}
	if detail == "" {
		return "ok"
	}
	return detail
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Summary returns a compact count string for tests and callers.
func Summary(checks []Check) string {
	errors := 0
	warnings := 0
	for _, check := range checks {
		if check.OK {
			continue
		}
		switch check.Severity {
		case SeverityError:
			errors++
		case SeverityWarn:
			warnings++
		}
	}
	return fmt.Sprintf("%d error(s), %d warning(s)", errors, warnings)
}
