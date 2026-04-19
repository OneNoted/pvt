use anyhow::{Context, Result, bail};
use std::{
    env,
    path::PathBuf,
    process::{Command, Stdio},
};

pub fn run(argv: &[String], max_output_bytes: usize) -> Result<String> {
    run_with_input(argv, None, max_output_bytes)
}

pub fn run_with_input(
    argv: &[String],
    input: Option<&str>,
    max_output_bytes: usize,
) -> Result<String> {
    if argv.is_empty() {
        bail!("empty command argv");
    }
    let mut command = Command::new(&argv[0]);
    command.args(&argv[1..]);
    if input.is_some() {
        command.stdin(Stdio::piped());
    } else {
        command.stdin(Stdio::null());
    }
    command.stdout(Stdio::piped());
    command.stderr(Stdio::piped());
    let mut child = command
        .spawn()
        .with_context(|| format!("failed to run {}", argv[0]))?;
    if let Some(input) = input
        && let Some(mut stdin) = child.stdin.take()
    {
        use std::io::Write as _;
        stdin
            .write_all(input.as_bytes())
            .with_context(|| format!("failed to write stdin for {}", argv[0]))?;
    }
    let output = child
        .wait_with_output()
        .with_context(|| format!("failed to wait for {}", argv[0]))?;
    if !output.status.success() {
        bail!(
            "command failed: {}",
            String::from_utf8_lossy(&output.stderr).trim().to_string()
        );
    }
    let mut stdout = output.stdout;
    if stdout.len() > max_output_bytes {
        stdout.truncate(max_output_bytes);
    }
    Ok(String::from_utf8_lossy(&stdout).into_owned())
}

pub fn resolve_binary(name: &str, env_override: &str) -> Result<String> {
    if let Some(path) = env::var_os(env_override) {
        let path = PathBuf::from(path);
        if path.is_file() {
            return Ok(path.to_string_lossy().into_owned());
        }
    }

    for directory in ["/usr/bin", "/bin", "/usr/local/bin", "/opt/homebrew/bin"] {
        let candidate = PathBuf::from(directory).join(name);
        if candidate.is_file() {
            return Ok(candidate.to_string_lossy().into_owned());
        }
    }

    bail!("unable to resolve binary for {name}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn captures_stdout_without_inheriting_terminal_io() {
        let shell = resolve_binary("sh", "PVT_SH_BIN").unwrap_or_else(|_| "/bin/sh".to_string());
        let argv = vec![
            shell,
            "-c".to_string(),
            "printf 'visible'; printf 'hidden' 1>&2".to_string(),
        ];
        let output = run(&argv, 1024).unwrap();
        assert_eq!(output, "visible");
    }

    #[test]
    fn supports_stdin_for_curl_config_style_commands() {
        let shell = resolve_binary("sh", "PVT_SH_BIN").unwrap_or_else(|_| "/bin/sh".to_string());
        let argv = vec![shell, "-c".to_string(), "cat".to_string()];
        let output = run_with_input(&argv, Some("hello"), 1024).unwrap();
        assert_eq!(output, "hello");
    }
}
