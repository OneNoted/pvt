use crate::config::ProxmoxCluster;
use crate::integrations::command;
use anyhow::{Context, Result};
use serde_json::Value;

#[derive(Debug, Clone, Default)]
pub struct VmStatus {
    pub vmid: i64,
    pub name: String,
    pub status: String,
    pub node: String,
    pub maxdisk: i64,
}

#[derive(Debug, Clone, Default)]
pub struct StoragePool {
    pub name: String,
    pub node: String,
    pub pool_type: String,
    pub status: String,
    pub disk: i64,
    pub maxdisk: i64,
}

#[derive(Debug, Clone, Default)]
pub struct NodeStatus {
    pub node: String,
    pub cpu: f64,
    pub mem: i64,
    pub maxmem: i64,
}

#[derive(Debug, Clone, Default)]
pub struct BackupEntry {
    pub volid: String,
    pub node: String,
    pub storage: String,
    pub size: i64,
    pub ctime: i64,
    pub vmid: i64,
}

pub fn get_cluster_resources(cluster: &ProxmoxCluster) -> Result<Vec<VmStatus>> {
    let body = request(cluster, "GET", "/api2/json/cluster/resources?type=vm")?;
    let items = data_array(&body)?;
    Ok(items
        .into_iter()
        .filter(|item| item.get("type").and_then(Value::as_str) == Some("qemu"))
        .map(|item| VmStatus {
            vmid: json_i64(&item, "vmid"),
            name: json_string(&item, "name", "unknown"),
            status: json_string(&item, "status", "unknown"),
            node: json_string(&item, "node", "unknown"),
            maxdisk: json_i64(&item, "maxdisk"),
        })
        .collect())
}

pub fn get_storage_pools(cluster: &ProxmoxCluster) -> Result<Vec<StoragePool>> {
    let body = request(cluster, "GET", "/api2/json/cluster/resources?type=storage")?;
    let items = data_array(&body)?;
    Ok(items
        .into_iter()
        .map(|item| StoragePool {
            name: json_string(&item, "storage", "unknown"),
            node: json_string(&item, "node", "unknown"),
            pool_type: item
                .get("plugintype")
                .and_then(Value::as_str)
                .or_else(|| item.get("type").and_then(Value::as_str))
                .unwrap_or("unknown")
                .to_string(),
            status: json_string(&item, "status", "unknown"),
            disk: json_i64(&item, "disk"),
            maxdisk: json_i64(&item, "maxdisk"),
        })
        .collect())
}

pub fn get_node_status(cluster: &ProxmoxCluster, node: &str) -> Result<NodeStatus> {
    let body = request(cluster, "GET", &format!("/api2/json/nodes/{node}/status"))?;
    let data = data_object(&body)?;
    Ok(NodeStatus {
        node: node.to_string(),
        cpu: json_f64(&data, "cpu"),
        mem: json_i64(&data, "mem"),
        maxmem: json_i64(&data, "maxmem"),
    })
}

pub fn list_backups(
    cluster: &ProxmoxCluster,
    node: &str,
    storage: &str,
) -> Result<Vec<BackupEntry>> {
    let body = request(
        cluster,
        "GET",
        &format!("/api2/json/nodes/{node}/storage/{storage}/content?content=backup"),
    )?;
    let items = data_array(&body)?;
    Ok(items
        .into_iter()
        .map(|item| BackupEntry {
            volid: json_string(&item, "volid", ""),
            node: node.to_string(),
            storage: storage.to_string(),
            size: json_i64(&item, "size"),
            ctime: json_i64(&item, "ctime"),
            vmid: json_i64(&item, "vmid"),
        })
        .collect())
}

pub fn delete_backup(
    cluster: &ProxmoxCluster,
    node: &str,
    storage: &str,
    volid: &str,
) -> Result<()> {
    let encoded = volid.replace(':', "%3A").replace('/', "%2F");
    let _ = request(
        cluster,
        "DELETE",
        &format!("/api2/json/nodes/{node}/storage/{storage}/content/{encoded}"),
    )?;
    Ok(())
}

fn request(cluster: &ProxmoxCluster, method: &str, path: &str) -> Result<String> {
    let curl = command::resolve_binary("curl", "PVT_CURL_BIN")?;
    let url = format!("{}{}", cluster.endpoint, path);
    let mut argv = vec![
        curl,
        "--silent".to_string(),
        "--show-error".to_string(),
        "--fail".to_string(),
        "--max-time".to_string(),
        "10".to_string(),
        "--config".to_string(),
        "-".to_string(),
    ];
    if method != "GET" {
        argv.push("-X".to_string());
        argv.push(method.to_string());
    }
    if !cluster.tls_verify {
        argv.push("-k".to_string());
    }
    let curl_config = format!(
        "url = \"{}\"\nheader = \"Authorization: PVEAPIToken={}={}\"\n",
        escape_curl_config(&url),
        escape_curl_config(&cluster.token_id),
        escape_curl_config(&cluster.token_secret),
    );
    command::run_with_input(&argv, Some(&curl_config), 1024 * 1024)
        .context("proxmox request failed")
}

fn escape_curl_config(value: &str) -> String {
    value.replace('\\', "\\\\").replace('"', "\\\"")
}

fn data_array(body: &str) -> Result<Vec<Value>> {
    let parsed: Value = serde_json::from_str(body).context("invalid proxmox JSON")?;
    Ok(parsed
        .get("data")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default())
}

fn data_object(body: &str) -> Result<Value> {
    let parsed: Value = serde_json::from_str(body).context("invalid proxmox JSON")?;
    Ok(parsed.get("data").cloned().unwrap_or(Value::Null))
}

fn json_string(value: &Value, key: &str, default: &str) -> String {
    value
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or(default)
        .to_string()
}

fn json_i64(value: &Value, key: &str) -> i64 {
    value
        .get(key)
        .and_then(|value| value.as_i64().or_else(|| value.as_f64().map(|v| v as i64)))
        .unwrap_or_default()
}

fn json_f64(value: &Value, key: &str) -> f64 {
    value
        .get(key)
        .and_then(|value| value.as_f64().or_else(|| value.as_i64().map(|v| v as f64)))
        .unwrap_or_default()
}
