package cluster

import (
	"context"
	"fmt"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/talos"
)

// ControlPlaneNodes returns only the control plane nodes, preserving order.
func ControlPlaneNodes(nodes []config.NodeConfig) []config.NodeConfig {
	var out []config.NodeConfig
	for _, n := range nodes {
		if n.Role == "controlplane" {
			out = append(out, n)
		}
	}
	return out
}

// WorkerNodes returns only the worker nodes, preserving order.
func WorkerNodes(nodes []config.NodeConfig) []config.NodeConfig {
	var out []config.NodeConfig
	for _, n := range nodes {
		if n.Role == "worker" {
			out = append(out, n)
		}
	}
	return out
}

// NodeIPs extracts IP addresses from a slice of nodes.
func NodeIPs(nodes []config.NodeConfig) []string {
	ips := make([]string, len(nodes))
	for i, n := range nodes {
		ips[i] = n.IP
	}
	return ips
}

// FindEtcdLeader identifies which control plane node is the current etcd leader.
func FindEtcdLeader(ctx context.Context, tc *talos.Client, cpNodes []config.NodeConfig) (config.NodeConfig, error) {
	cpIPs := NodeIPs(cpNodes)

	statuses, err := tc.EtcdStatus(ctx, cpIPs...)
	if err != nil {
		return config.NodeConfig{}, fmt.Errorf("querying etcd status: %w", err)
	}

	for _, s := range statuses {
		if s.IsLeader {
			// Match leader back to a node config by IP or hostname
			for _, n := range cpNodes {
				if n.IP == s.Node || n.Name == s.Node {
					return n, nil
				}
			}
		}
	}

	return config.NodeConfig{}, fmt.Errorf("could not determine etcd leader")
}

// OrderForUpgrade returns control plane nodes ordered so the etcd leader is last.
// If the leader cannot be determined, the original order is preserved.
func OrderForUpgrade(ctx context.Context, tc *talos.Client, cpNodes []config.NodeConfig) ([]config.NodeConfig, error) {
	if len(cpNodes) <= 1 {
		return cpNodes, nil
	}

	leader, err := FindEtcdLeader(ctx, tc, cpNodes)
	if err != nil {
		return cpNodes, err
	}

	var ordered []config.NodeConfig
	for _, n := range cpNodes {
		if n.IP != leader.IP {
			ordered = append(ordered, n)
		}
	}
	ordered = append(ordered, leader)

	return ordered, nil
}
