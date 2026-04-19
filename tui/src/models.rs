#[derive(Debug, Clone, Default)]
pub struct Snapshot {
    pub cluster_rows: Vec<ClusterRow>,
    pub storage_pools: Vec<StoragePoolRow>,
    pub vm_disks: Vec<VmDiskRow>,
    pub backups: Vec<BackupRow>,
    pub k8s_backups: Vec<K8sBackupRow>,
    pub hosts: Vec<HostRow>,
    pub pods: Vec<PodMetricRow>,
    pub metrics_available: bool,
    pub loading: bool,
    pub last_refresh_label: Option<String>,
    pub last_error: Option<String>,
}

#[derive(Debug, Clone)]
pub struct ClusterRow {
    pub name: String,
    pub role: String,
    pub ip: String,
    pub pve_node: String,
    pub vmid: String,
    pub talos_version: String,
    pub kubernetes_version: String,
    pub etcd: String,
    pub health: String,
}

#[derive(Debug, Clone)]
pub struct StoragePoolRow {
    pub name: String,
    pub node: String,
    pub pool_type: String,
    pub used_str: String,
    pub total_str: String,
    pub status: String,
    pub usage_pct: f64,
}

#[derive(Debug, Clone)]
pub struct VmDiskRow {
    pub vm_name: String,
    pub vmid: String,
    pub node: String,
    pub size_str: String,
    pub size_bytes: i64,
}

#[derive(Debug, Clone)]
pub struct BackupRow {
    pub proxmox_cluster: String,
    pub volid: String,
    pub node: String,
    pub storage: String,
    pub vm_name: String,
    pub vmid: String,
    pub size_str: String,
    pub date_str: String,
    pub age_days: u32,
    pub is_stale: bool,
}

#[derive(Debug, Clone)]
pub struct K8sBackupRow {
    pub name: String,
    pub namespace: String,
    pub source_type: String,
    pub status: String,
    pub schedule: String,
    pub last_run: String,
}

#[derive(Debug, Clone)]
pub struct HostRow {
    pub name: String,
    pub cpu_pct: f64,
    pub mem_used_str: String,
    pub mem_total_str: String,
    pub mem_pct: f64,
}

#[derive(Debug, Clone)]
pub struct PodMetricRow {
    pub pod: String,
    pub namespace: String,
    pub cpu_str: String,
    pub mem_str: String,
    pub net_rx_str: String,
    pub net_tx_str: String,
    pub cpu_cores: f64,
    pub mem_bytes: f64,
    pub net_rx_bytes_sec: f64,
    pub net_tx_bytes_sec: f64,
}

#[derive(Debug, Clone)]
pub struct DeleteAction {
    pub proxmox_cluster: String,
    pub node: String,
    pub storage: String,
    pub volid: String,
}
