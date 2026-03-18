package talos

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"google.golang.org/grpc"

	clusterapi "github.com/siderolabs/talos/pkg/machinery/api/cluster"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
)

// NodeVersion holds Talos and Kubernetes version info for a node.
type NodeVersion struct {
	Node         string // hostname from metadata (often the IP)
	Endpoint     string // the raw metadata hostname (IP or name)
	TalosVersion string
	K8sVersion   string
	Arch         string
}

// NodeService represents a running Talos service on a node.
type NodeService struct {
	ID      string
	State   string
	Healthy bool
}

// Version returns Talos and K8s version info for the targeted node(s).
func (c *Client) Version(ctx context.Context, nodes ...string) ([]NodeVersion, error) {
	if len(nodes) > 0 {
		ctx = talosclient.WithNodes(ctx, nodes...)
	}

	resp, err := c.api.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting version: %w", err)
	}

	var versions []NodeVersion
	for _, msg := range resp.Messages {
		nv := NodeVersion{
			Node:     msg.Metadata.GetHostname(),
			Endpoint: msg.Metadata.GetHostname(),
		}
		if msg.Version != nil {
			nv.TalosVersion = msg.Version.Tag
			nv.Arch = msg.Version.Arch
		}
		versions = append(versions, nv)
	}

	return versions, nil
}

// ServiceList returns the list of services running on a node.
func (c *Client) ServiceList(ctx context.Context, nodes ...string) ([]NodeService, error) {
	if len(nodes) > 0 {
		ctx = talosclient.WithNodes(ctx, nodes...)
	}

	resp, err := c.api.ServiceList(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing services: %w", err)
	}

	var services []NodeService
	for _, msg := range resp.Messages {
		for _, svc := range msg.Services {
			services = append(services, NodeService{
				ID:      svc.Id,
				State:   svc.State,
				Healthy: svc.Health.Healthy,
			})
		}
	}

	return services, nil
}

// EtcdMembers returns the list of etcd members.
func (c *Client) EtcdMembers(ctx context.Context, nodes ...string) ([]EtcdMember, error) {
	if len(nodes) > 0 {
		ctx = talosclient.WithNodes(ctx, nodes...)
	}

	resp, err := c.api.EtcdMemberList(ctx, &machineapi.EtcdMemberListRequest{
		QueryLocal: false,
	})
	if err != nil {
		return nil, fmt.Errorf("listing etcd members: %w", err)
	}

	var members []EtcdMember
	for _, msg := range resp.Messages {
		for _, m := range msg.Members {
			members = append(members, EtcdMember{
				ID:         m.Id,
				Hostname:   m.Hostname,
				PeerURLs:   m.PeerUrls,
				ClientURLs: m.ClientUrls,
				IsLearner:  m.IsLearner,
			})
		}
	}

	return members, nil
}

// EtcdStatus returns the etcd status for the targeted node.
func (c *Client) EtcdStatus(ctx context.Context, nodes ...string) ([]EtcdNodeStatus, error) {
	if len(nodes) > 0 {
		ctx = talosclient.WithNodes(ctx, nodes...)
	}

	resp, err := c.api.EtcdStatus(ctx, grpc.EmptyCallOption{})
	if err != nil {
		return nil, fmt.Errorf("getting etcd status: %w", err)
	}

	var statuses []EtcdNodeStatus
	for _, msg := range resp.Messages {
		ms := msg.GetMemberStatus()
		statuses = append(statuses, EtcdNodeStatus{
			Node:     msg.Metadata.GetHostname(),
			MemberID: ms.GetMemberId(),
			IsLeader: ms.GetMemberId() == ms.GetLeader(),
			DBSize:   ms.GetDbSize(),
		})
	}

	return statuses, nil
}

// ApplyConfig applies a Talos machine configuration to a node.
func (c *Client) ApplyConfig(ctx context.Context, nodeIP string, configData []byte, mode machineapi.ApplyConfigurationRequest_Mode) error {
	ctx = talosclient.WithNodes(ctx, nodeIP)

	_, err := c.api.ApplyConfiguration(ctx, &machineapi.ApplyConfigurationRequest{
		Data: configData,
		Mode: mode,
	})
	if err != nil {
		return fmt.Errorf("applying config to %s: %w", nodeIP, err)
	}

	return nil
}

// BootstrapEtcd bootstraps etcd on the target node.
func (c *Client) BootstrapEtcd(ctx context.Context, nodeIP string) error {
	ctx = talosclient.WithNodes(ctx, nodeIP)

	err := c.api.Bootstrap(ctx, &machineapi.BootstrapRequest{})
	if err != nil {
		return fmt.Errorf("bootstrapping etcd on %s: %w", nodeIP, err)
	}

	return nil
}

// UpgradeNode upgrades Talos on the target node.
func (c *Client) UpgradeNode(ctx context.Context, nodeIP string, image string, stage, force bool) error {
	ctx = talosclient.WithNodes(ctx, nodeIP)

	_, err := c.api.Upgrade(ctx, image, stage, force)
	if err != nil {
		return fmt.Errorf("upgrading %s: %w", nodeIP, err)
	}

	return nil
}

// EtcdForfeitLeadership asks the target node to give up etcd leadership.
func (c *Client) EtcdForfeitLeadership(ctx context.Context, nodeIP string) error {
	ctx = talosclient.WithNodes(ctx, nodeIP)

	_, err := c.api.EtcdForfeitLeadership(ctx, &machineapi.EtcdForfeitLeadershipRequest{})
	if err != nil {
		return fmt.Errorf("forfeiting etcd leadership on %s: %w", nodeIP, err)
	}

	return nil
}

// EtcdSnapshot takes an etcd snapshot from the target node and writes it to w.
func (c *Client) EtcdSnapshot(ctx context.Context, nodeIP string, w io.Writer) error {
	ctx = talosclient.WithNodes(ctx, nodeIP)

	r, err := c.api.EtcdSnapshot(ctx, &machineapi.EtcdSnapshotRequest{})
	if err != nil {
		return fmt.Errorf("taking etcd snapshot from %s: %w", nodeIP, err)
	}
	defer r.Close()

	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("writing etcd snapshot: %w", err)
	}

	return nil
}

// WaitHealthy runs a cluster health check and blocks until the cluster is
// healthy or the timeout expires. Progress messages are printed to stderr.
func (c *Client) WaitHealthy(ctx context.Context, cpIPs, workerIPs []string, timeout time.Duration) error {
	stream, err := c.api.ClusterHealthCheck(ctx, timeout, &clusterapi.ClusterInfo{
		ControlPlaneNodes: cpIPs,
		WorkerNodes:       workerIPs,
	})
	if err != nil {
		return fmt.Errorf("starting health check: %w", err)
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("health check failed: %w", err)
		}

		if msg.GetMessage() != "" {
			fmt.Fprintf(os.Stderr, "  Health: %s\n", msg.GetMessage())
		}
	}
}
