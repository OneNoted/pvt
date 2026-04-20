package health

import (
	"context"
	"fmt"
	"sort"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/proxmox"
	"github.com/OneNoted/pvt/internal/rules"
	"github.com/OneNoted/pvt/internal/talos"
)

// Snapshot is a best-effort view of configured and live cluster state.
type Snapshot struct {
	ConfigPath string
	Clusters   []ClusterSnapshot
	Errors     []string
}

// ClusterSnapshot is the live state for one configured cluster.
type ClusterSnapshot struct {
	Name           string
	Endpoint       string
	ProxmoxCluster string
	Nodes          []NodeSnapshot
	Errors         []string
}

// NodeSnapshot is the live state for one configured node.
type NodeSnapshot struct {
	Config             config.NodeConfig
	VMStatus           string
	VMConfig           *proxmox.VMConfig
	TalosVersion       string
	TalosVersionSource string
	ValidationFindings []rules.Finding
	Errors             []string
}

// Gather builds a best-effort health snapshot from config and live APIs.
func Gather(ctx context.Context, configPath string, cfg *config.Config) Snapshot {
	snapshot := Snapshot{ConfigPath: configPath}
	pxByName := make(map[string]config.ProxmoxCluster)
	for _, px := range cfg.Proxmox.Clusters {
		pxByName[px.Name] = px
	}

	registry := rules.DefaultRegistry()
	for _, clusterCfg := range cfg.Clusters {
		clusterSnap := ClusterSnapshot{
			Name:           clusterCfg.Name,
			Endpoint:       clusterCfg.Endpoint,
			ProxmoxCluster: clusterCfg.ProxmoxCluster,
		}

		versions := talosVersionsByNode(ctx, cfg, clusterCfg, &clusterSnap)
		pxCfg, ok := pxByName[clusterCfg.ProxmoxCluster]
		var pxClient *proxmox.Client
		if ok {
			client, err := proxmox.NewClient(pxCfg)
			if err != nil {
				clusterSnap.Errors = append(clusterSnap.Errors, fmt.Sprintf("proxmox client: %v", err))
			} else {
				pxClient = client
			}
		} else {
			clusterSnap.Errors = append(clusterSnap.Errors, fmt.Sprintf("unknown proxmox cluster %q", clusterCfg.ProxmoxCluster))
		}

		for _, node := range clusterCfg.Nodes {
			nodeSnap := NodeSnapshot{Config: node}
			if version, ok := versions[node.Name]; ok {
				nodeSnap.TalosVersion = version.TalosVersion
				nodeSnap.TalosVersionSource = version.Node
			} else if version, ok := versions[node.IP]; ok {
				nodeSnap.TalosVersion = version.TalosVersion
				nodeSnap.TalosVersionSource = version.Node
			}

			if pxClient != nil {
				vmCfg, err := pxClient.GetVMConfig(ctx, node.ProxmoxNode, node.ProxmoxVMID)
				if err != nil {
					nodeSnap.Errors = append(nodeSnap.Errors, fmt.Sprintf("vm config: %v", err))
				} else {
					nodeSnap.VMConfig = vmCfg
					nodeSnap.VMStatus = "present"
					nodeSnap.ValidationFindings = registry.Validate(vmCfg)
				}
				if summaries, err := pxClient.ListNodeVMs(ctx, node.ProxmoxNode); err == nil {
					for _, summary := range summaries {
						if summary.VMID == node.ProxmoxVMID {
							nodeSnap.VMStatus = summary.Status
							break
						}
					}
				}
			}

			clusterSnap.Nodes = append(clusterSnap.Nodes, nodeSnap)
		}
		sort.Slice(clusterSnap.Nodes, func(i, j int) bool {
			return clusterSnap.Nodes[i].Config.Name < clusterSnap.Nodes[j].Config.Name
		})
		snapshot.Clusters = append(snapshot.Clusters, clusterSnap)
	}
	return snapshot
}

func talosVersionsByNode(ctx context.Context, cfg *config.Config, clusterCfg config.ClusterConfig, clusterSnap *ClusterSnapshot) map[string]talos.NodeVersion {
	cpEndpoints := []string{}
	allNodes := []string{}
	for _, node := range clusterCfg.Nodes {
		allNodes = append(allNodes, node.IP)
		if node.Role == "controlplane" {
			cpEndpoints = append(cpEndpoints, node.IP)
		}
	}
	if len(cpEndpoints) == 0 {
		clusterSnap.Errors = append(clusterSnap.Errors, "no control plane nodes configured for Talos query")
		return nil
	}

	client, err := talos.NewClient(ctx, cfg.Talos.ConfigPath, cfg.Talos.Context, cpEndpoints)
	if err != nil {
		clusterSnap.Errors = append(clusterSnap.Errors, fmt.Sprintf("talos client: %v", err))
		return nil
	}
	defer client.Close()

	versions, err := client.Version(ctx, allNodes...)
	if err != nil {
		clusterSnap.Errors = append(clusterSnap.Errors, fmt.Sprintf("talos versions: %v", err))
		return nil
	}

	out := make(map[string]talos.NodeVersion)
	for _, version := range versions {
		out[version.Node] = version
		out[version.Endpoint] = version
	}
	return out
}
