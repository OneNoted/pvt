use crate::config::Config;
use crate::integrations::{kubernetes, metrics::MetricsClient, proxmox, talos};
use crate::models::{
    BackupRow, ClusterRow, DeleteAction, HostRow, K8sBackupRow, PodMetricRow, Snapshot,
    StoragePoolRow, VmDiskRow,
};
use crate::util::{age_days, format_bytes, format_epoch, format_rate, format_system_time};
use std::collections::{BTreeSet, HashMap};
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, Sender};
use std::thread::{self, JoinHandle};
use std::time::SystemTime;

pub enum PollerCommand {
    RefreshNow,
    DeleteBackup(DeleteAction),
    Shutdown,
}

pub struct PollerHandle {
    pub snapshots: Receiver<Snapshot>,
    pub commands: Sender<PollerCommand>,
    join: Option<JoinHandle<()>>,
}

impl PollerHandle {
    pub fn spawn(config: Config) -> Self {
        let (snapshot_tx, snapshot_rx) = mpsc::channel();
        let (command_tx, command_rx) = mpsc::channel();
        let join = thread::spawn(move || run_loop(config, snapshot_tx, command_rx));
        Self {
            snapshots: snapshot_rx,
            commands: command_tx,
            join: Some(join),
        }
    }
}

impl Drop for PollerHandle {
    fn drop(&mut self) {
        let _ = self.commands.send(PollerCommand::Shutdown);
        if let Some(join) = self.join.take() {
            let _ = join.join();
        }
    }
}

fn run_loop(config: Config, snapshots: Sender<Snapshot>, commands: Receiver<PollerCommand>) {
    let _ = snapshots.send(refresh_snapshot(&config, None));
    let interval = config.tui.refresh_interval;
    loop {
        match commands.recv_timeout(interval) {
            Ok(PollerCommand::RefreshNow) => {
                let _ = snapshots.send(refresh_snapshot(&config, None));
            }
            Ok(PollerCommand::DeleteBackup(action)) => {
                let outcome = delete_backup(&config, &action)
                    .err()
                    .map(|err| err.to_string());
                let _ = snapshots.send(refresh_snapshot(&config, outcome));
            }
            Ok(PollerCommand::Shutdown) => break,
            Err(RecvTimeoutError::Timeout) => {
                let _ = snapshots.send(refresh_snapshot(&config, None));
            }
            Err(RecvTimeoutError::Disconnected) => break,
        }
    }
}

fn delete_backup(config: &Config, action: &DeleteAction) -> anyhow::Result<()> {
    let Some(cluster) = config
        .proxmox
        .clusters
        .iter()
        .find(|cluster| cluster.name == action.proxmox_cluster)
    else {
        anyhow::bail!("unknown proxmox cluster {}", action.proxmox_cluster);
    };
    proxmox::delete_backup(cluster, &action.node, &action.storage, &action.volid)
}

pub fn refresh_snapshot(config: &Config, command_error: Option<String>) -> Snapshot {
    let mut errors = Vec::new();
    if let Some(error) = command_error {
        errors.push(error);
    }

    let mut snapshot = Snapshot {
        loading: false,
        ..Snapshot::default()
    };
    let mut discovered_node_ips: HashMap<String, String> = HashMap::new();
    let mut discovered_node_roles: HashMap<String, String> = HashMap::new();

    let mut vm_resources_by_cluster: HashMap<String, Vec<proxmox::VmStatus>> = HashMap::new();
    let mut storage_by_cluster: HashMap<String, Vec<proxmox::StoragePool>> = HashMap::new();
    let mut node_status_by_cluster: HashMap<String, HashMap<String, proxmox::NodeStatus>> =
        HashMap::new();

    for cluster in &config.proxmox.clusters {
        match proxmox::get_cluster_resources(cluster) {
            Ok(resources) => {
                vm_resources_by_cluster.insert(cluster.name.clone(), resources);
            }
            Err(err) => errors.push(format!("{} VM resources: {err}", cluster.name)),
        }
        match proxmox::get_storage_pools(cluster) {
            Ok(storage) => {
                storage_by_cluster.insert(cluster.name.clone(), storage);
            }
            Err(err) => errors.push(format!("{} storage pools: {err}", cluster.name)),
        }

        let mut node_statuses = HashMap::new();
        let unique_nodes = config
            .clusters
            .iter()
            .filter(|managed| managed.proxmox_cluster == cluster.name)
            .flat_map(|managed| managed.nodes.iter().map(|node| node.proxmox_node.clone()))
            .collect::<BTreeSet<_>>();
        for node in unique_nodes {
            if let Ok(status) = proxmox::get_node_status(cluster, &node) {
                node_statuses.insert(node, status);
            }
        }
        node_status_by_cluster.insert(cluster.name.clone(), node_statuses);
    }

    match kubernetes::get_cluster_nodes() {
        Ok(nodes) => {
            for node in nodes {
                if let Some(ip) = node.internal_ip {
                    discovered_node_ips.insert(node.name.clone(), ip);
                }
                discovered_node_roles.insert(node.name, node.role);
            }
        }
        Err(err) => errors.push(format!("cluster nodes: {err}")),
    }

    let etcd_members = talos::get_etcd_members(&config.talos).unwrap_or_else(|err| {
        errors.push(format!("etcd members: {err}"));
        Vec::new()
    });
    let etcd_map = etcd_members
        .into_iter()
        .map(|member| {
            (
                member.hostname,
                if member.is_learner {
                    "learner".to_string()
                } else {
                    "member".to_string()
                },
            )
        })
        .collect::<HashMap<_, _>>();

    let mut configured_vmids = BTreeSet::new();
    let mut configured_names = BTreeSet::new();
    for managed_cluster in &config.clusters {
        let resources = vm_resources_by_cluster
            .get(&managed_cluster.proxmox_cluster)
            .cloned()
            .unwrap_or_default();
        let resource_by_vmid = resources
            .iter()
            .map(|resource| (resource.vmid, resource.clone()))
            .collect::<HashMap<_, _>>();

        for node in &managed_cluster.nodes {
            configured_vmids.insert(node.proxmox_vmid);
            configured_names.insert(node.name.clone());
            let live_ip = discovered_node_ips
                .get(&node.name)
                .cloned()
                .unwrap_or_else(|| node.ip.clone());
            let talos_version = talos::get_version(&config.talos, &live_ip).ok();
            let vm = resource_by_vmid.get(&node.proxmox_vmid);
            let health = match (vm.map(|vm| vm.status.as_str()), talos_version.as_ref()) {
                (Some("running"), Some(_)) => "healthy",
                (Some("running"), None) => "degraded",
                (Some(_), _) => "stopped",
                (None, Some(_)) => "unknown-vm",
                (None, None) => "unknown",
            };
            snapshot.cluster_rows.push(ClusterRow {
                name: node.name.clone(),
                role: node.role.clone(),
                ip: live_ip,
                pve_node: node.proxmox_node.clone(),
                vmid: node.proxmox_vmid.to_string(),
                talos_version: talos_version
                    .as_ref()
                    .map(|version| version.talos_version.clone())
                    .unwrap_or_else(|| "-".to_string()),
                kubernetes_version: talos_version
                    .as_ref()
                    .map(|version| version.kubernetes_version.clone())
                    .unwrap_or_else(|| "-".to_string()),
                etcd: etcd_map
                    .get(&node.name)
                    .cloned()
                    .unwrap_or_else(|| "-".to_string()),
                health: health.to_string(),
            });
        }
    }

    for resources in vm_resources_by_cluster.values() {
        for resource in resources {
            if configured_vmids.contains(&resource.vmid)
                || configured_names.contains(&resource.name)
            {
                continue;
            }
            let Some(ip) = discovered_node_ips.get(&resource.name).cloned() else {
                continue;
            };
            let talos_version = talos::get_version(&config.talos, &ip).ok();
            let health = match (resource.status.as_str(), talos_version.as_ref()) {
                ("running", Some(_)) => "healthy",
                ("running", None) => "degraded",
                (_, Some(_)) => "unknown",
                _ => "unknown",
            };
            snapshot.cluster_rows.push(ClusterRow {
                name: resource.name.clone(),
                role: discovered_node_roles
                    .get(&resource.name)
                    .cloned()
                    .unwrap_or_else(|| infer_role_from_name(&resource.name)),
                ip,
                pve_node: resource.node.clone(),
                vmid: resource.vmid.to_string(),
                talos_version: talos_version
                    .as_ref()
                    .map(|version| version.talos_version.clone())
                    .unwrap_or_else(|| "-".to_string()),
                kubernetes_version: talos_version
                    .as_ref()
                    .map(|version| version.kubernetes_version.clone())
                    .unwrap_or_else(|| "-".to_string()),
                etcd: etcd_map
                    .get(&resource.name)
                    .cloned()
                    .unwrap_or_else(|| "-".to_string()),
                health: health.to_string(),
            });
        }
    }

    for cluster in &config.proxmox.clusters {
        let resources = vm_resources_by_cluster
            .get(&cluster.name)
            .cloned()
            .unwrap_or_default();
        for resource in &resources {
            snapshot.vm_disks.push(VmDiskRow {
                vm_name: resource.name.clone(),
                vmid: resource.vmid.to_string(),
                node: resource.node.clone(),
                size_str: format_bytes(resource.maxdisk),
                size_bytes: resource.maxdisk,
            });
        }
        if let Some(storage) = storage_by_cluster.get(&cluster.name) {
            for pool in storage {
                let used = pool.disk.max(0);
                let total = pool.maxdisk.max(0);
                let usage_pct = if total == 0 {
                    0.0
                } else {
                    (used as f64 / total as f64) * 100.0
                };
                snapshot.storage_pools.push(StoragePoolRow {
                    name: pool.name.clone(),
                    node: pool.node.clone(),
                    pool_type: pool.pool_type.clone(),
                    used_str: format_bytes(used),
                    total_str: format_bytes(total),
                    status: pool.status.clone(),
                    usage_pct,
                });

                match proxmox::list_backups(cluster, &pool.node, &pool.name) {
                    Ok(backups) => {
                        for backup in backups {
                            let vm_name = resources
                                .iter()
                                .find(|resource| resource.vmid == backup.vmid)
                                .map(|resource| resource.name.clone())
                                .unwrap_or_else(|| "unknown".to_string());
                            let age = age_days(backup.ctime);
                            snapshot.backups.push(BackupRow {
                                proxmox_cluster: cluster.name.clone(),
                                volid: backup.volid.clone(),
                                node: backup.node.clone(),
                                storage: backup.storage.clone(),
                                vm_name,
                                vmid: backup.vmid.to_string(),
                                size_str: format_bytes(backup.size),
                                date_str: format_epoch(backup.ctime),
                                age_days: age,
                                is_stale: age >= config.tui.backups.stale_days,
                            });
                        }
                    }
                    Err(err) => errors.push(format!(
                        "{} backups {}/{}: {err}",
                        cluster.name, pool.node, pool.name
                    )),
                }
            }
        }
    }

    match kubernetes::detect_providers() {
        Ok(providers) => {
            if providers.volsync {
                match kubernetes::get_volsync_sources() {
                    Ok(entries) => snapshot
                        .k8s_backups
                        .extend(entries.into_iter().map(k8s_row)),
                    Err(err) => errors.push(format!("volsync backups: {err}")),
                }
            }
            if providers.velero {
                match kubernetes::get_velero_backups() {
                    Ok(entries) => snapshot
                        .k8s_backups
                        .extend(entries.into_iter().map(k8s_row)),
                    Err(err) => errors.push(format!("velero backups: {err}")),
                }
            }
        }
        Err(err) => errors.push(format!("kubernetes providers: {err}")),
    }

    for (cluster_name, statuses) in &node_status_by_cluster {
        for status in statuses.values() {
            snapshot.hosts.push(HostRow {
                name: format!("{cluster_name}/{}", status.node),
                cpu_pct: status.cpu * 100.0,
                mem_used_str: format_bytes(status.mem),
                mem_total_str: format_bytes(status.maxmem),
                mem_pct: if status.maxmem == 0 {
                    0.0
                } else {
                    (status.mem as f64 / status.maxmem as f64) * 100.0
                },
            });
        }
    }

    match MetricsClient::detect() {
        Ok(Some(client)) => {
            snapshot.metrics_available = true;
            let cpu = client
                .query("sum(rate(container_cpu_usage_seconds_total{container!=\"\",pod!=\"\"}[5m])) by (pod, namespace)")
                .unwrap_or_else(|err| {
                    errors.push(format!("metrics cpu: {err}"));
                    Vec::new()
                });
            let mem = client
                .query("sum(container_memory_working_set_bytes{container!=\"\",pod!=\"\"}) by (pod, namespace)")
                .unwrap_or_default();
            let rx = client
                .query("sum(rate(container_network_receive_bytes_total{pod!=\"\"}[5m])) by (pod, namespace)")
                .unwrap_or_default();
            let tx = client
                .query("sum(rate(container_network_transmit_bytes_total{pod!=\"\"}[5m])) by (pod, namespace)")
                .unwrap_or_default();

            let mem_map = metric_map(mem);
            let rx_map = metric_map(rx);
            let tx_map = metric_map(tx);
            for sample in cpu {
                let pod = label(&sample.labels, "pod");
                let namespace = label(&sample.labels, "namespace");
                let key = format!("{namespace}/{pod}");
                let mem_value = mem_map.get(&key).copied().unwrap_or_default();
                let rx_value = rx_map.get(&key).copied().unwrap_or_default();
                let tx_value = tx_map.get(&key).copied().unwrap_or_default();
                snapshot.pods.push(PodMetricRow {
                    pod,
                    namespace,
                    cpu_str: format!("{:.3}", sample.value),
                    mem_str: format_bytes(mem_value as i64),
                    net_rx_str: format_rate(rx_value),
                    net_tx_str: format_rate(tx_value),
                    cpu_cores: sample.value,
                    mem_bytes: mem_value,
                    net_rx_bytes_sec: rx_value,
                    net_tx_bytes_sec: tx_value,
                });
            }
        }
        Ok(None) => {
            snapshot.metrics_available = false;
        }
        Err(err) => errors.push(format!("metrics detect: {err}")),
    }

    snapshot.cluster_rows.sort_by(|a, b| a.name.cmp(&b.name));
    snapshot.storage_pools.sort_by(|a, b| a.name.cmp(&b.name));
    snapshot
        .vm_disks
        .sort_by(|a, b| b.size_bytes.cmp(&a.size_bytes));
    snapshot.backups.sort_by(|a, b| b.age_days.cmp(&a.age_days));
    snapshot
        .k8s_backups
        .sort_by(|a, b| a.namespace.cmp(&b.namespace).then(a.name.cmp(&b.name)));
    snapshot.hosts.sort_by(|a, b| a.name.cmp(&b.name));
    snapshot.pods.sort_by(|a, b| {
        b.cpu_cores
            .partial_cmp(&a.cpu_cores)
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    snapshot.last_refresh_label = Some(format_system_time(SystemTime::now()));
    if !errors.is_empty() {
        snapshot.last_error = Some(errors.join(" | "));
    }
    snapshot
}

fn infer_role_from_name(name: &str) -> String {
    let lower = name.to_ascii_lowercase();
    if lower.contains("control") || lower.contains("-cp-") || lower.contains("master") {
        "controlplane".to_string()
    } else {
        "worker".to_string()
    }
}

fn k8s_row(entry: kubernetes::K8sBackupEntry) -> K8sBackupRow {
    K8sBackupRow {
        name: entry.name,
        namespace: entry.namespace,
        source_type: entry.source_type,
        status: entry.status,
        schedule: entry.schedule,
        last_run: entry.last_run,
    }
}

fn metric_map(samples: Vec<crate::integrations::metrics::MetricSample>) -> HashMap<String, f64> {
    samples
        .into_iter()
        .map(|sample| {
            let pod = label(&sample.labels, "pod");
            let namespace = label(&sample.labels, "namespace");
            (format!("{namespace}/{pod}"), sample.value)
        })
        .collect()
}

fn label(labels: &serde_json::Map<String, serde_json::Value>, key: &str) -> String {
    labels
        .get(key)
        .and_then(|value| value.as_str())
        .unwrap_or("unknown")
        .to_string()
}
