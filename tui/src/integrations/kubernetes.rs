use crate::integrations::command;
use anyhow::Result;
use serde_json::Value;
use std::{env, path::PathBuf};

#[derive(Debug, Clone, Default)]
pub struct K8sBackupEntry {
    pub name: String,
    pub namespace: String,
    pub source_type: String,
    pub status: String,
    pub schedule: String,
    pub last_run: String,
}

#[derive(Debug, Clone, Default)]
pub struct DetectedProviders {
    pub volsync: bool,
    pub velero: bool,
}

#[derive(Debug, Clone, Default)]
pub struct ClusterNode {
    pub name: String,
    pub internal_ip: Option<String>,
    pub role: String,
}

pub fn detect_providers() -> Result<DetectedProviders> {
    let output = kubectl(&[
        "get",
        "crd",
        "--no-headers",
        "-o",
        "custom-columns=NAME:.metadata.name",
    ])?;
    let mut providers = DetectedProviders::default();
    for line in output.lines() {
        let line = line.trim();
        if line.contains("volsync") {
            providers.volsync = true;
        }
        if line.contains("velero") {
            providers.velero = true;
        }
    }
    Ok(providers)
}

pub fn get_volsync_sources() -> Result<Vec<K8sBackupEntry>> {
    parse_backups(
        &kubectl(&[
            "get",
            "replicationsources.volsync.backube",
            "-A",
            "-o",
            "json",
        ])?,
        "VolSync",
    )
}

pub fn get_velero_backups() -> Result<Vec<K8sBackupEntry>> {
    parse_backups(
        &kubectl(&["get", "backups.velero.io", "-A", "-o", "json"])?,
        "Velero",
    )
}

pub fn get_cluster_nodes() -> Result<Vec<ClusterNode>> {
    let parsed: Value = serde_json::from_str(&kubectl(&["get", "nodes", "-o", "json"])?)?;
    let items = parsed
        .get("items")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    Ok(items
        .into_iter()
        .map(|item| {
            let metadata = item.get("metadata").cloned().unwrap_or(Value::Null);
            let status = item.get("status").cloned().unwrap_or(Value::Null);
            let labels = metadata
                .get("labels")
                .and_then(Value::as_object)
                .cloned()
                .unwrap_or_default();
            let internal_ip =
                status
                    .get("addresses")
                    .and_then(Value::as_array)
                    .and_then(|addresses| {
                        addresses.iter().find_map(|address| {
                            (address.get("type").and_then(Value::as_str) == Some("InternalIP"))
                                .then(|| {
                                    address
                                        .get("address")
                                        .and_then(Value::as_str)
                                        .map(ToOwned::to_owned)
                                })
                                .flatten()
                        })
                    });
            let role = if labels.contains_key("node-role.kubernetes.io/control-plane")
                || labels.contains_key("node-role.kubernetes.io/master")
            {
                "controlplane".to_string()
            } else {
                "worker".to_string()
            };

            ClusterNode {
                name: metadata
                    .get("name")
                    .and_then(Value::as_str)
                    .unwrap_or("unknown")
                    .to_string(),
                internal_ip,
                role,
            }
        })
        .collect())
}

fn parse_backups(body: &str, source_type: &str) -> Result<Vec<K8sBackupEntry>> {
    let parsed: Value = serde_json::from_str(body)?;
    let items = parsed
        .get("items")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    Ok(items
        .into_iter()
        .map(|item| {
            let metadata = item.get("metadata").cloned().unwrap_or(Value::Null);
            let status = item.get("status").cloned().unwrap_or(Value::Null);
            let spec = item.get("spec").cloned().unwrap_or(Value::Null);
            K8sBackupEntry {
                name: metadata
                    .get("name")
                    .and_then(Value::as_str)
                    .unwrap_or("unknown")
                    .to_string(),
                namespace: metadata
                    .get("namespace")
                    .and_then(Value::as_str)
                    .unwrap_or("default")
                    .to_string(),
                source_type: source_type.to_string(),
                status: if source_type == "VolSync" {
                    parse_volsync_status(&status)
                } else {
                    status
                        .get("phase")
                        .and_then(Value::as_str)
                        .unwrap_or("unknown")
                        .to_string()
                },
                schedule: if source_type == "VolSync" {
                    spec.get("trigger")
                        .and_then(|value| value.get("schedule"))
                        .and_then(Value::as_str)
                        .unwrap_or("-")
                        .to_string()
                } else {
                    spec.get("scheduleName")
                        .and_then(Value::as_str)
                        .unwrap_or("-")
                        .to_string()
                },
                last_run: if source_type == "VolSync" {
                    status
                        .get("lastSyncTime")
                        .and_then(Value::as_str)
                        .unwrap_or("-")
                        .to_string()
                } else {
                    status
                        .get("completionTimestamp")
                        .and_then(Value::as_str)
                        .unwrap_or("-")
                        .to_string()
                },
            }
        })
        .collect())
}

fn parse_volsync_status(status: &Value) -> String {
    let conditions = status
        .get("conditions")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    for condition in conditions {
        if condition.get("type").and_then(Value::as_str) == Some("Synchronizing") {
            return if condition.get("status").and_then(Value::as_str) == Some("True") {
                "Syncing".to_string()
            } else {
                "Idle".to_string()
            };
        }
    }
    "unknown".to_string()
}

fn kubectl(extra: &[&str]) -> Result<String> {
    let kubectl = command::resolve_binary("kubectl", "PVT_KUBECTL_BIN")?;
    let mut argv = vec![kubectl];
    argv.extend(extra.iter().map(|value| value.to_string()));
    if let Some(kubeconfig) = discover_kubeconfig() {
        argv.push("--kubeconfig".to_string());
        argv.push(kubeconfig);
    }
    command::run(&argv, 512 * 1024)
}

pub(crate) fn discover_kubeconfig() -> Option<String> {
    if let Some(path) = env::var_os("KUBECONFIG") {
        let path = PathBuf::from(path);
        if path.is_file() {
            return Some(path.to_string_lossy().into_owned());
        }
    }

    let home = env::var_os("HOME")?;
    let candidate = PathBuf::from(home).join(".kube/config");
    if candidate.is_file() {
        return Some(candidate.to_string_lossy().into_owned());
    }
    None
}
