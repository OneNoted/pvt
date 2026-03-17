package config

import "time"

// Config is the top-level pvt configuration.
type Config struct {
	Version  string          `yaml:"version" mapstructure:"version"`
	Proxmox  ProxmoxConfig   `yaml:"proxmox" mapstructure:"proxmox"`
	Talos    TalosConfig     `yaml:"talos" mapstructure:"talos"`
	Clusters []ClusterConfig `yaml:"clusters" mapstructure:"clusters"`
}

// ProxmoxConfig holds Proxmox VE connection settings.
type ProxmoxConfig struct {
	Clusters []ProxmoxCluster `yaml:"clusters" mapstructure:"clusters"`
}

// ProxmoxCluster represents a single Proxmox VE cluster connection.
type ProxmoxCluster struct {
	Name        string `yaml:"name" mapstructure:"name"`
	Endpoint    string `yaml:"endpoint" mapstructure:"endpoint"`
	TokenID     string `yaml:"token_id" mapstructure:"token_id"`
	TokenSecret string `yaml:"token_secret" mapstructure:"token_secret"`
	TLSVerify   bool   `yaml:"tls_verify" mapstructure:"tls_verify"`
}

// TalosConfig holds Talos connection defaults.
type TalosConfig struct {
	ConfigPath string `yaml:"config_path" mapstructure:"config_path"`
	Context    string `yaml:"context" mapstructure:"context"`
}

// ClusterConfig represents a managed Talos-on-Proxmox cluster.
type ClusterConfig struct {
	Name           string           `yaml:"name" mapstructure:"name"`
	ProxmoxCluster string           `yaml:"proxmox_cluster" mapstructure:"proxmox_cluster"`
	Endpoint       string           `yaml:"endpoint" mapstructure:"endpoint"`
	ConfigSource   ConfigSource     `yaml:"config_source" mapstructure:"config_source"`
	Nodes          []NodeConfig     `yaml:"nodes" mapstructure:"nodes"`
	Validation     ValidationConfig `yaml:"validation" mapstructure:"validation"`
	Upgrade        UpgradeConfig    `yaml:"upgrade" mapstructure:"upgrade"`
}

// ConfigSource describes where Talos machine configs come from.
type ConfigSource struct {
	Type string `yaml:"type" mapstructure:"type"` // "directory" or "talhelper"
	Path string `yaml:"path" mapstructure:"path"`
}

// NodeConfig represents a single node in the cluster.
type NodeConfig struct {
	Name        string `yaml:"name" mapstructure:"name"`
	Role        string `yaml:"role" mapstructure:"role"` // "controlplane" or "worker"
	ProxmoxVMID int    `yaml:"proxmox_vmid" mapstructure:"proxmox_vmid"`
	ProxmoxNode string `yaml:"proxmox_node" mapstructure:"proxmox_node"`
	IP          string `yaml:"ip" mapstructure:"ip"`
}

// ValidationConfig holds per-cluster validation rule overrides.
type ValidationConfig struct {
	Rules map[string]RuleConfig `yaml:"rules" mapstructure:"rules"`
}

// RuleConfig is a per-rule configuration override.
type RuleConfig struct {
	Enabled *bool    `yaml:"enabled,omitempty" mapstructure:"enabled"`
	Allowed []string `yaml:"allowed,omitempty" mapstructure:"allowed"`
}

// UpgradeConfig holds upgrade orchestration settings.
type UpgradeConfig struct {
	EtcdBackupBefore   bool          `yaml:"etcd_backup_before" mapstructure:"etcd_backup_before"`
	HealthCheckTimeout time.Duration `yaml:"health_check_timeout" mapstructure:"health_check_timeout"`
	PauseBetweenNodes  time.Duration `yaml:"pause_between_nodes" mapstructure:"pause_between_nodes"`
}
