package drift

import (
	"testing"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/health"
	"github.com/OneNoted/pvt/internal/rules"
)

func TestDetectIncludesValidationAndRuntimeFindings(t *testing.T) {
	snapshot := health.Snapshot{
		Clusters: []health.ClusterSnapshot{{
			Name: "lab",
			Nodes: []health.NodeSnapshot{{
				Config:       config.NodeConfig{Name: "cp-1", IP: "10.0.0.10", ProxmoxVMID: 100},
				VMStatus:     "stopped",
				TalosVersion: "",
				ValidationFindings: []rules.Finding{{
					Rule:     "cpu-type",
					Severity: rules.SeverityError,
					Message:  "CPU type is wrong",
					Fix:      "qm set 100 -cpu host",
				}},
			}},
		}},
	}

	findings := Detect(snapshot)
	if len(findings) != 3 {
		t.Fatalf("Detect() produced %d findings, want 3", len(findings))
	}
	if !HasErrors(findings) {
		t.Fatal("HasErrors() = false, want true")
	}
	remediations := Remediations(findings)
	if len(remediations) != 2 {
		t.Fatalf("Remediations() produced %d findings, want 2", len(remediations))
	}
}
