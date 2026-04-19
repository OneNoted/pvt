use crate::integrations::command;
use anyhow::Result;
use serde_json::Value;

#[derive(Debug, Clone, Default)]
pub struct MetricSample {
    pub labels: serde_json::Map<String, Value>,
    pub value: f64,
}

#[derive(Debug, Clone)]
pub struct MetricsClient {
    endpoint: String,
}

impl MetricsClient {
    pub fn detect() -> Result<Option<Self>> {
        let kubectl = command::resolve_binary("kubectl", "PVT_KUBECTL_BIN")?;
        let candidates = [
            (
                "monitoring",
                "vmsingle-victoria-metrics-victoria-metrics-single-server",
                "8428",
            ),
            ("monitoring", "vmselect", "8481"),
            ("monitoring", "prometheus-server", "9090"),
            ("monitoring", "prometheus-operated", "9090"),
            ("observability", "prometheus-server", "9090"),
            ("observability", "prometheus-operated", "9090"),
        ];
        for (namespace, service, port) in candidates {
            let mut argv = vec![
                kubectl.clone(),
                "get".to_string(),
                "svc".to_string(),
                service.to_string(),
                "-n".to_string(),
                namespace.to_string(),
                "--no-headers".to_string(),
                "-o".to_string(),
                "name".to_string(),
            ];
            if let Some(kubeconfig) = super::kubernetes::discover_kubeconfig() {
                argv.push("--kubeconfig".to_string());
                argv.push(kubeconfig);
            }
            if command::run(&argv, 4096).is_ok() {
                return Ok(Some(Self {
                    endpoint: format!("http://{service}.{namespace}.svc:{port}"),
                }));
            }
        }
        Ok(None)
    }

    pub fn query(&self, query: &str) -> Result<Vec<MetricSample>> {
        let curl = command::resolve_binary("curl", "PVT_CURL_BIN")?;
        let argv = vec![
            curl,
            "-s".to_string(),
            "-f".to_string(),
            "--max-time".to_string(),
            "5".to_string(),
            "--get".to_string(),
            "--data-urlencode".to_string(),
            format!("query={query}"),
            format!("{}/api/v1/query", self.endpoint),
        ];
        let body = command::run(&argv, 1024 * 1024)?;
        let parsed: Value = serde_json::from_str(&body)?;
        let results = parsed
            .get("data")
            .and_then(|value| value.get("result"))
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        Ok(results
            .into_iter()
            .filter_map(|item| {
                let labels = item.get("metric")?.as_object()?.clone();
                let value = item
                    .get("value")?
                    .as_array()?
                    .get(1)?
                    .as_str()?
                    .parse::<f64>()
                    .ok()?;
                Some(MetricSample { labels, value })
            })
            .collect())
    }
}
