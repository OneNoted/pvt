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

## Usage

```bash
pvt config init            # generate starter config
pvt config validate        # validate config syntax
pvt status summary         # per-node cluster overview
pvt validate vms           # pre-flight VM checks
pvt validate vm <name>     # check a single VM
pvt bootstrap              # apply machine configs + bootstrap etcd
pvt upgrade --image <img>  # rolling Talos upgrade across all nodes
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
      tls_verify: false

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
- [ ] Node lifecycle management
- [ ] Drift detection
- [ ] TUI dashboard
