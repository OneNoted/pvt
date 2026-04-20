use anyhow::{Context, Result, anyhow, bail};
use serde::Deserialize;
use std::{
    collections::HashSet,
    env, fs,
    path::{Path, PathBuf},
    time::Duration,
};

#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    pub version: String,
    pub proxmox: ProxmoxConfig,
    pub talos: TalosConfig,
    pub clusters: Vec<ClusterConfig>,
    #[serde(default)]
    pub tui: TuiConfig,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ProxmoxConfig {
    pub clusters: Vec<ProxmoxCluster>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ProxmoxCluster {
    pub name: String,
    pub endpoint: String,
    pub token_id: String,
    pub token_secret: String,
    #[serde(default = "default_tls_verify")]
    pub tls_verify: bool,
}

#[derive(Debug, Clone, Deserialize)]
pub struct TalosConfig {
    pub config_path: String,
    #[serde(default)]
    pub context: String,
}

#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
pub struct ClusterConfig {
    pub name: String,
    pub proxmox_cluster: String,
    pub endpoint: String,
    #[serde(default)]
    pub config_source: Option<ConfigSource>,
    pub nodes: Vec<NodeConfig>,
    #[serde(default)]
    pub validation: ValidationConfig,
    #[serde(default)]
    pub upgrade: UpgradeConfig,
}

#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
pub struct ConfigSource {
    pub r#type: String,
    pub path: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct NodeConfig {
    pub name: String,
    pub role: String,
    pub proxmox_vmid: i64,
    pub proxmox_node: String,
    pub ip: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[allow(dead_code)]
pub struct ValidationConfig {
    #[serde(default)]
    pub rules: std::collections::BTreeMap<String, RuleConfig>,
}

#[derive(Debug, Clone, Default, Deserialize)]
#[allow(dead_code)]
pub struct RuleConfig {
    #[serde(default)]
    pub enabled: Option<bool>,
    #[serde(default)]
    pub allowed: Vec<String>,
}

#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
pub struct UpgradeConfig {
    #[serde(default = "default_etcd_backup_before")]
    pub etcd_backup_before: bool,
    #[serde(
        default = "default_health_check_timeout",
        deserialize_with = "deserialize_duration"
    )]
    pub health_check_timeout: Duration,
    #[serde(
        default = "default_pause_between_nodes",
        deserialize_with = "deserialize_duration"
    )]
    pub pause_between_nodes: Duration,
}

#[derive(Debug, Clone, Deserialize)]
pub struct TuiConfig {
    #[serde(default)]
    pub storage: StorageTuiConfig,
    #[serde(default)]
    pub backups: BackupsTuiConfig,
    #[serde(
        default = "default_refresh_interval",
        deserialize_with = "deserialize_duration"
    )]
    pub refresh_interval: Duration,
}

#[derive(Debug, Clone, Deserialize)]
pub struct StorageTuiConfig {
    #[serde(default = "default_warn_threshold")]
    pub warn_threshold: u8,
    #[serde(default = "default_crit_threshold")]
    pub crit_threshold: u8,
}

#[derive(Debug, Clone, Deserialize)]
pub struct BackupsTuiConfig {
    #[serde(default = "default_stale_days")]
    pub stale_days: u32,
}

impl Default for UpgradeConfig {
    fn default() -> Self {
        Self {
            etcd_backup_before: default_etcd_backup_before(),
            health_check_timeout: default_health_check_timeout(),
            pause_between_nodes: default_pause_between_nodes(),
        }
    }
}

impl Default for TuiConfig {
    fn default() -> Self {
        Self {
            storage: StorageTuiConfig::default(),
            backups: BackupsTuiConfig::default(),
            refresh_interval: default_refresh_interval(),
        }
    }
}

impl Default for StorageTuiConfig {
    fn default() -> Self {
        Self {
            warn_threshold: default_warn_threshold(),
            crit_threshold: default_crit_threshold(),
        }
    }
}

impl Default for BackupsTuiConfig {
    fn default() -> Self {
        Self {
            stale_days: default_stale_days(),
        }
    }
}

pub fn parse_args() -> Result<Option<PathBuf>> {
    let mut args = env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "-c" | "--config" => {
                let Some(path) = args.next() else {
                    bail!("--config requires a path argument");
                };
                return Ok(Some(PathBuf::from(path)));
            }
            "-h" | "--help" => {
                print_help();
                return Ok(None);
            }
            _ => {}
        }
    }
    Ok(Some(discover_config()?))
}

pub fn print_help() {
    println!("vitui - TUI for pvt cluster management\n");
    println!("Usage: vitui [options]\n");
    println!("Options:");
    println!("  -c, --config <path>  Path to pvt.yaml config file");
    println!("  -h, --help           Show this help message\n");
    println!("Discovery order:");
    println!("  $PVT_CONFIG, ./pvt.yaml, ~/.config/pvt/config.yaml, ~/.config/pvt/pvt.yaml");
}

pub fn load_from_path(path: &Path) -> Result<Config> {
    let raw = fs::read_to_string(path)
        .with_context(|| format!("failed to read config file {}", path.display()))?;
    let expanded = expand_env_vars(&raw)?;
    let mut cfg: Config = serde_yaml::from_str(&expanded).context("failed to parse YAML config")?;
    normalize_paths(&mut cfg);
    validate(&cfg)?;
    Ok(cfg)
}

pub fn discover_config() -> Result<PathBuf> {
    if let Ok(path) = env::var("PVT_CONFIG") {
        let path = PathBuf::from(path);
        if path.exists() {
            return Ok(path);
        }
    }

    let local = PathBuf::from("pvt.yaml");
    if local.exists() {
        return local
            .canonicalize()
            .or_else(|_| Ok::<PathBuf, std::io::Error>(local.clone()))
            .context("failed to resolve local pvt.yaml path");
    }

    let Some(home) = home_dir() else {
        bail!(
            "no pvt config file found (searched: $PVT_CONFIG, ./pvt.yaml, ~/.config/pvt/config.yaml, ~/.config/pvt/pvt.yaml)"
        );
    };
    for candidate in [
        home.join(".config/pvt/config.yaml"),
        home.join(".config/pvt/pvt.yaml"),
    ] {
        if candidate.exists() {
            return Ok(candidate);
        }
    }

    bail!(
        "no pvt config file found (searched: $PVT_CONFIG, ./pvt.yaml, ~/.config/pvt/config.yaml, ~/.config/pvt/pvt.yaml)"
    )
}

fn normalize_paths(cfg: &mut Config) {
    cfg.talos.config_path = expand_tilde(&cfg.talos.config_path);
    for cluster in &mut cfg.clusters {
        if let Some(source) = &mut cluster.config_source {
            source.path = expand_tilde(&source.path);
        }
    }
}

fn validate(cfg: &Config) -> Result<()> {
    if cfg.version.trim().is_empty() {
        bail!("config: version is required");
    }
    if cfg.version != "1" {
        bail!(
            "config: unsupported version {:?} (supported: \"1\")",
            cfg.version
        );
    }
    if cfg.proxmox.clusters.is_empty() {
        bail!("config: at least one proxmox cluster must be defined");
    }
    for (index, cluster) in cfg.proxmox.clusters.iter().enumerate() {
        if cluster.name.trim().is_empty() {
            bail!("config: proxmox.clusters[{index}].name is required");
        }
        if cluster.endpoint.trim().is_empty() {
            bail!("config: proxmox.clusters[{index}].endpoint is required");
        }
    }

    if cfg.clusters.is_empty() {
        bail!("config: at least one cluster must be defined");
    }
    let pve_names = cfg
        .proxmox
        .clusters
        .iter()
        .map(|cluster| cluster.name.as_str())
        .collect::<HashSet<_>>();
    for (index, cluster) in cfg.clusters.iter().enumerate() {
        if cluster.name.trim().is_empty() {
            bail!("config: clusters[{index}].name is required");
        }
        if cluster.proxmox_cluster.trim().is_empty() {
            bail!("config: clusters[{index}].proxmox_cluster is required");
        }
        if cluster.endpoint.trim().is_empty() {
            bail!("config: clusters[{index}].endpoint is required");
        }
        if cluster.nodes.is_empty() {
            bail!("config: clusters[{index}].nodes must not be empty");
        }
        if !pve_names.contains(cluster.proxmox_cluster.as_str()) {
            bail!(
                "config: clusters[{index}].proxmox_cluster {:?} does not match any defined proxmox cluster",
                cluster.proxmox_cluster
            );
        }
        for (node_index, node) in cluster.nodes.iter().enumerate() {
            if node.name.trim().is_empty() {
                bail!("config: clusters[{index}].nodes[{node_index}].name is required");
            }
            if !matches!(node.role.as_str(), "controlplane" | "worker") {
                bail!(
                    "config: clusters[{index}].nodes[{node_index}].role must be \"controlplane\" or \"worker\", got {:?}",
                    node.role
                );
            }
            if node.proxmox_vmid == 0 {
                bail!("config: clusters[{index}].nodes[{node_index}].proxmox_vmid is required");
            }
            if node.proxmox_node.trim().is_empty() {
                bail!("config: clusters[{index}].nodes[{node_index}].proxmox_node is required");
            }
            if node.ip.trim().is_empty() {
                bail!("config: clusters[{index}].nodes[{node_index}].ip is required");
            }
        }
    }

    Ok(())
}

pub fn expand_env_vars(input: &str) -> Result<String> {
    let mut out = String::with_capacity(input.len());
    let chars = input.as_bytes();
    let mut index = 0;
    while index < chars.len() {
        if chars[index] == b'$' {
            if chars.get(index + 1) == Some(&b'{') {
                let start = index + 2;
                let Some(end) = input[start..].find('}') else {
                    return Err(anyhow!("unterminated environment variable reference"));
                };
                let end = start + end;
                let name = &input[start..end];
                let value = env::var(name).unwrap_or_default();
                out.push_str(&value);
                index = end + 1;
                continue;
            }

            let start = index + 1;
            let mut end = start;
            while end < chars.len()
                && matches!(chars[end], b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'_')
            {
                end += 1;
            }
            if end > start {
                let name = &input[start..end];
                let value = env::var(name).unwrap_or_default();
                out.push_str(&value);
                index = end;
                continue;
            }
        }
        out.push(chars[index] as char);
        index += 1;
    }
    Ok(out)
}

pub fn parse_duration(input: &str) -> Result<Duration> {
    if input.is_empty() {
        return Ok(default_refresh_interval());
    }
    let (value, unit) = input.split_at(input.len() - 1);
    let amount = value
        .parse::<u64>()
        .with_context(|| format!("invalid duration value: {input}"))?;
    match unit {
        "s" => Ok(Duration::from_secs(amount)),
        "m" => Ok(Duration::from_secs(amount * 60)),
        "h" => Ok(Duration::from_secs(amount * 60 * 60)),
        _ => bail!("unsupported duration unit in {input}"),
    }
}

fn deserialize_duration<'de, D>(deserializer: D) -> std::result::Result<Duration, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let value = String::deserialize(deserializer)?;
    parse_duration(&value).map_err(serde::de::Error::custom)
}

fn expand_tilde(value: &str) -> String {
    if let Some(stripped) = value.strip_prefix('~')
        && let Some(home) = home_dir()
    {
        return format!("{}{}", home.display(), stripped);
    }
    value.to_string()
}

fn home_dir() -> Option<PathBuf> {
    env::var_os("HOME").map(PathBuf::from)
}

fn default_tls_verify() -> bool {
    true
}
fn default_etcd_backup_before() -> bool {
    true
}
fn default_health_check_timeout() -> Duration {
    Duration::from_secs(300)
}
fn default_pause_between_nodes() -> Duration {
    Duration::from_secs(30)
}
fn default_warn_threshold() -> u8 {
    10
}
fn default_crit_threshold() -> u8 {
    5
}
fn default_stale_days() -> u32 {
    30
}
fn default_refresh_interval() -> Duration {
    Duration::from_secs(30)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    const VALID_CONFIG: &str = r#"
version: "1"
proxmox:
  clusters:
    - name: homelab
      endpoint: https://proxmox.local:8006
      token_id: pvt@pam!automation
      token_secret: ${PVT_TOKEN}
      tls_verify: false
talos:
  config_path: ~/.talos/config
  context: prod
clusters:
  - name: prod
    proxmox_cluster: homelab
    endpoint: https://192.168.1.100:6443
    config_source:
      type: directory
      path: ~/talos/prod
    nodes:
      - name: cp-1
        role: controlplane
        proxmox_vmid: 100
        proxmox_node: pve1
        ip: 192.168.1.100
"#;

    #[test]
    fn expands_env_vars_to_empty_like_go_loader() {
        let parsed = expand_env_vars("token: ${MISSING_VAR}").unwrap();
        assert_eq!(parsed, "token: ");
        let parsed = expand_env_vars("token: $MISSING_VAR").unwrap();
        assert_eq!(parsed, "token: ");
    }

    #[test]
    fn parses_duration_values() {
        assert_eq!(parse_duration("30s").unwrap(), Duration::from_secs(30));
        assert_eq!(parse_duration("5m").unwrap(), Duration::from_secs(300));
        assert!(parse_duration("oops").is_err());
    }

    #[test]
    fn loads_and_normalizes_paths() {
        unsafe {
            env::set_var("PVT_TOKEN", "secret");
        }
        let temp = TempDir::new().unwrap();
        let mut path = temp.path().to_path_buf();
        path.push("config.yaml");
        fs::write(&path, VALID_CONFIG).unwrap();
        let cfg = load_from_path(&path).unwrap();
        assert_eq!(cfg.version, "1");
        assert!(cfg.talos.config_path.contains('/'));
        assert_eq!(cfg.proxmox.clusters[0].token_secret, "secret");
    }

    #[test]
    fn rejects_invalid_role() {
        let invalid = VALID_CONFIG.replace("controlplane", "master");
        let temp = TempDir::new().unwrap();
        let mut path = temp.path().to_path_buf();
        path.push("config.yaml");
        fs::write(&path, invalid).unwrap();
        let error = load_from_path(&path).unwrap_err().to_string();
        assert!(error.contains("controlplane"));
    }

    #[test]
    fn discovers_legacy_home_fallback() {
        let temp = TempDir::new().unwrap();
        let home = temp.path();
        let legacy = home.join(".config/pvt/pvt.yaml");
        fs::create_dir_all(legacy.parent().unwrap()).unwrap();
        fs::write(&legacy, "version: \"1\"\n").unwrap();
        unsafe {
            env::remove_var("PVT_CONFIG");
            env::set_var("HOME", home);
        }
        std::env::set_current_dir(temp.path()).unwrap();
        let found = discover_config().unwrap();
        assert_eq!(found, legacy);
    }
}
