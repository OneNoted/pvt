use crate::config::TalosConfig;
use crate::integrations::command;
use anyhow::Result;
use serde_json::Value;

#[derive(Debug, Clone, Default)]
pub struct TalosVersion {
    pub talos_version: String,
    pub kubernetes_version: String,
}

#[derive(Debug, Clone, Default)]
pub struct EtcdMember {
    pub hostname: String,
    pub is_learner: bool,
}

pub fn get_version(talos: &TalosConfig, node_ip: &str) -> Result<TalosVersion> {
    let argv = talosctl_args(
        talos,
        vec![
            "version".to_string(),
            "--nodes".to_string(),
            node_ip.to_string(),
            "--short".to_string(),
        ],
    )?;
    let output = command::run(&argv, 512 * 1024)?;
    let parsed: Value = serde_json::from_str(&output)?;
    let message = parsed
        .get("messages")
        .and_then(Value::as_array)
        .and_then(|items| items.first())
        .and_then(Value::as_object)
        .ok_or_else(|| anyhow::anyhow!("missing talos version payload"))?;
    let version = message
        .get("version")
        .and_then(Value::as_object)
        .ok_or_else(|| anyhow::anyhow!("missing version object"))?;
    Ok(TalosVersion {
        talos_version: version
            .get("tag")
            .and_then(Value::as_str)
            .unwrap_or("unknown")
            .to_string(),
        kubernetes_version: version
            .get("kubernetes_version")
            .and_then(Value::as_str)
            .unwrap_or("-")
            .to_string(),
    })
}

pub fn get_etcd_members(talos: &TalosConfig) -> Result<Vec<EtcdMember>> {
    let argv = talosctl_args(talos, vec!["etcd".to_string(), "members".to_string()])?;
    let output = command::run(&argv, 512 * 1024)?;
    let parsed: Value = serde_json::from_str(&output)?;
    let members = parsed
        .get("messages")
        .and_then(Value::as_array)
        .and_then(|items| items.first())
        .and_then(|item| item.get("members"))
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    Ok(members
        .into_iter()
        .map(|item| EtcdMember {
            hostname: item
                .get("hostname")
                .and_then(Value::as_str)
                .unwrap_or("unknown")
                .to_string(),
            is_learner: item
                .get("is_learner")
                .and_then(Value::as_bool)
                .unwrap_or(false),
        })
        .collect())
}

fn talosctl_args(talos: &TalosConfig, extra: Vec<String>) -> Result<Vec<String>> {
    let binary = command::resolve_binary("talosctl", "PVT_TALOSCTL_BIN")?;
    let mut argv = vec![binary];
    argv.extend(extra);
    argv.push("--talosconfig".to_string());
    argv.push(talos.config_path.clone());
    if !talos.context.is_empty() {
        argv.push("--context".to_string());
        argv.push(talos.context.clone());
    }
    argv.push("-o".to_string());
    argv.push("json".to_string());
    Ok(argv)
}
