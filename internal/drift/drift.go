package drift

import (
	"fmt"
	"sort"

	"github.com/OneNoted/pvt/internal/health"
	"github.com/OneNoted/pvt/internal/rules"
)

// Finding describes a detected config/live-state mismatch or risk.
type Finding struct {
	Cluster  string
	Node     string
	Severity rules.Severity
	Kind     string
	Message  string
	Fix      string
}

// Detect extracts drift findings from a health snapshot.
func Detect(snapshot health.Snapshot) []Finding {
	var findings []Finding
	for _, cluster := range snapshot.Clusters {
		for _, err := range cluster.Errors {
			findings = append(findings, Finding{
				Cluster:  cluster.Name,
				Severity: rules.SeverityWarn,
				Kind:     "cluster",
				Message:  err,
			})
		}
		for _, node := range cluster.Nodes {
			if node.VMStatus != "" && node.VMStatus != "running" {
				findings = append(findings, Finding{
					Cluster:  cluster.Name,
					Node:     node.Config.Name,
					Severity: rules.SeverityError,
					Kind:     "vm-status",
					Message:  fmt.Sprintf("VM %d is %s", node.Config.ProxmoxVMID, node.VMStatus),
					Fix:      fmt.Sprintf("qm start %d", node.Config.ProxmoxVMID),
				})
			}
			if node.TalosVersion == "" {
				findings = append(findings, Finding{
					Cluster:  cluster.Name,
					Node:     node.Config.Name,
					Severity: rules.SeverityWarn,
					Kind:     "talos-reachability",
					Message:  fmt.Sprintf("Talos version unavailable for %s", node.Config.IP),
				})
			}
			for _, err := range node.Errors {
				findings = append(findings, Finding{
					Cluster:  cluster.Name,
					Node:     node.Config.Name,
					Severity: rules.SeverityWarn,
					Kind:     "observation",
					Message:  err,
				})
			}
			for _, validation := range node.ValidationFindings {
				findings = append(findings, Finding{
					Cluster:  cluster.Name,
					Node:     node.Config.Name,
					Severity: validation.Severity,
					Kind:     validation.Rule,
					Message:  validation.Message,
					Fix:      validation.Fix,
				})
			}
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Cluster != findings[j].Cluster {
			return findings[i].Cluster < findings[j].Cluster
		}
		if findings[i].Node != findings[j].Node {
			return findings[i].Node < findings[j].Node
		}
		return findings[i].Kind < findings[j].Kind
	})
	return findings
}

// HasErrors reports whether findings include error-level drift.
func HasErrors(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Severity == rules.SeverityError {
			return true
		}
	}
	return false
}

// Remediations returns known fix commands.
func Remediations(findings []Finding) []Finding {
	out := []Finding{}
	for _, finding := range findings {
		if finding.Fix != "" {
			out = append(out, finding)
		}
	}
	return out
}
