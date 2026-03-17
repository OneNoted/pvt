package talos

// EtcdMember represents an etcd cluster member.
type EtcdMember struct {
	ID         uint64
	Hostname   string
	PeerURLs   []string
	ClientURLs []string
	IsLearner  bool
}

// EtcdNodeStatus represents etcd status for a single node.
type EtcdNodeStatus struct {
	Node     string
	MemberID uint64
	IsLeader bool
	DBSize   int64
}
