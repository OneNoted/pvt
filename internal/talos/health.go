package talos

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	talosclient "github.com/siderolabs/talos/pkg/machinery/client"
)

// NodeVersion holds Talos and Kubernetes version info for a node.
type NodeVersion struct {
	Node       string
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
			Node: msg.Metadata.GetHostname(),
		}
		if msg.Version != nil {
			nv.TalosVersion = msg.Version.Tag
			nv.Arch = msg.Version.Arch
		}
		if msg.Platform != nil {
			// K8s version reported here when available
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
