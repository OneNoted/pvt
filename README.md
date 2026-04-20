<p align="center">
  <h1 align="center">pvt</h1>
  <p align="center">Proxmox VE + Talos Linux cluster lifecycle CLI</p>
</p>

<p align="center">
  <a href="https://github.com/OneNoted/pvt/actions"><img src="https://img.shields.io/github/actions/workflow/status/OneNoted/pvt/ci.yaml?branch=main&style=flat-square" alt="CI"></a>
  <a href="https://github.com/OneNoted/pvt/releases"><img src="https://img.shields.io/github/v/release/OneNoted/pvt?style=flat-square" alt="Release"></a>
  <a href="https://goreportcard.com/report/github.com/OneNoted/pvt"><img src="https://goreportcard.com/badge/github.com/OneNoted/pvt?style=flat-square" alt="Go Report Card"></a>
  <img src="https://img.shields.io/github/license/OneNoted/pvt?style=flat-square" alt="License">
  <img src="https://img.shields.io/github/go-mod/go-version/OneNoted/pvt?style=flat-square" alt="Go Version">
</p>

---

Pre-flight validation, cluster status, and lifecycle orchestration for Talos clusters on Proxmox VE.

## Install

```bash
go install github.com/OneNoted/pvt@latest
```

## TUI build

The interactive `pvt tui` dashboard now uses a Rust + Ratatui binary named `vitui`.

Build it from the repository root with:

```bash
cd tui
cargo build --release
```

`pvt tui` will look for `vitui` next to the `pvt` binary, then in `tui/target/release/`
and `tui/target/debug/` relative to the current working directory, the `pvt` binary
directory, and its parent directory. Set `PVT_VITUI_BIN` to use an explicit binary path.

If your system installs helper binaries outside standard locations, you can also override:

- `PVT_KUBECTL_BIN`
- `PVT_TALOSCTL_BIN`
- `PVT_CURL_BIN`

## Usage

```bash
pvt config init            # generate starter config
pvt config validate        # validate config syntax
pvt doctor                 # diagnose local config, helper tools, and API access
pvt status summary         # per-node cluster overview
pvt drift                  # compare pvt.yaml with live Proxmox/Talos state
pvt plan remediate         # print known remediation commands for drift
pvt validate vms           # pre-flight VM checks
pvt validate vm <name>     # check a single VM
pvt backups stale          # list stale Proxmox backups
pvt node reboot <name>     # plan a safe node reboot
pvt machineconfig diff --against <dir> # normalized Talos machine config diff
pvt bootstrap              # apply machine configs + bootstrap etcd
pvt upgrade --image <img>  # rolling Talos upgrade across all nodes
```

### Doctor, Drift, and Plans

`pvt doctor` checks config discovery, config parsing, helper binaries, Talos and
Kubernetes config files, and Proxmox API reachability. `pvt drift` uses the same
Go health snapshot engine as `pvt status summary` to surface VM, Talos, and
validation drift. `pvt plan remediate` prints known fix commands, but does not
apply them.

```bash
pvt doctor
pvt drift
pvt plan remediate
```

### Node Lifecycle

Node lifecycle commands are plan-first. `drain` and `reboot` can be run with
`--execute`; `add`, `replace`, and `remove` print the ordered operational plan
for review.

```bash
pvt node drain worker-1
pvt node reboot worker-1 --execute
pvt node replace old-worker --replacement new-worker
```

### Backups

The backups commands inspect Proxmox storage that supports backup content and
only include backups whose VMID matches a node in `pvt.yaml`. Pruning is a dry
run unless `--execute` is provided.

```bash
pvt backups list
pvt backups stale --older-than-days 30
pvt backups prune --older-than-days 30
```

### Bootstrap

Applies Talos machine configs and bootstraps etcd for a new cluster. Nodes must already be booted with the Talos ISO in maintenance mode.

```bash
pvt bootstrap                    # bootstrap the configured cluster
pvt bootstrap my-cluster         # target a specific cluster
pvt bootstrap --dry-run          # preview the plan without executing
```

Machine configs are resolved from the `config_source` setting — either a directory of `<node-name>.yaml` files or talhelper's `clusterconfig/` output.

### Rolling Upgrades

Upgrades Talos on all nodes one at a time: workers first, then control plane nodes with the etcd leader last.

```bash
pvt upgrade --image ghcr.io/siderolabs/installer:v1.12.5
pvt upgrade preflight --image ghcr.io/siderolabs/installer:v1.12.5
pvt upgrade postflight --image ghcr.io/siderolabs/installer:v1.12.5
pvt upgrade --image <img> --dry-run    # preview upgrade plan
pvt upgrade --image <img> --stage      # stage upgrade, reboot later
pvt upgrade --image <img> --force      # skip pre-flight health check
```

Respects `upgrade` settings from the config: `etcd_backup_before`, `health_check_timeout`, `pause_between_nodes`.

## Configuration

Reads from `./pvt.yaml`, `~/.config/pvt/config.yaml`, or `$PVT_CONFIG`. Supports `${ENV_VAR}` expansion.

```yaml
version: "1"

proxmox:
  clusters:
    - name: homelab
      endpoint: "https://pve.local:8006"
      token_id: "pvt@pam!automation"
      token_secret: "${PVT_PVE_TOKEN}"
      tls_verify: false  # only for self-signed lab setups; prefer true

talos:
  config_path: "~/talos/mycluster/talosconfig"
  context: "mycluster"

clusters:
  - name: mycluster
    proxmox_cluster: homelab
    endpoint: "https://192.168.1.100:6443"
    nodes:
      - name: cp-1
        role: controlplane
        proxmox_vmid: 100
        proxmox_node: pve1
        ip: "192.168.1.100"
```

## Validation Rules

| Rule | Severity | Description |
|------|----------|-------------|
| `cpu-type` | `ERROR` | Must be `host` — `kvm64` lacks required x86-64-v2 |
| `scsihw` | `ERROR` | Must be `virtio-scsi-pci` |
| `memory-min` | `ERROR` | Minimum 2048 MiB |
| `balloon` | `WARN` | Should be `0` — Talos has no balloon agent |
| `network-model` | `WARN` | Should be `virtio` |
| `qemu-agent` | `WARN` | Should be enabled |
| `machine-type` | `INFO` | `q35` recommended |
| `serial-console` | `INFO` | Recommended for boot debugging |

Findings include the corresponding `qm set` fix command.

## Roadmap

- [x] Pre-flight VM validation
- [x] Cluster status overview
- [x] Bootstrap orchestration
- [x] Rolling upgrades
- [x] Node lifecycle management
- [x] Drift detection
- [x] Rust Ratatui TUI dashboard
