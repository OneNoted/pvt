const std = @import("std");
const config = @import("config.zig");
const proxmox = @import("api/proxmox.zig");
const talos = @import("api/talos.zig");
const kubernetes = @import("api/kubernetes.zig");
const metrics_api = @import("api/metrics.zig");
const Allocator = std.mem.Allocator;

/// A single row in the cluster view table.
/// All fields are display-ready strings.
pub const NodeRow = struct {
    name: []const u8,
    role: []const u8,
    ip: []const u8,
    pve_node: []const u8,
    vmid: []const u8,
    talos_ver: []const u8,
    k8s_ver: []const u8,
    etcd: []const u8,
    health: []const u8,
};

/// A single row in the storage pools table.
pub const StoragePoolRow = struct {
    name: []const u8,
    node: []const u8,
    pool_type: []const u8,
    used_str: []const u8,
    total_str: []const u8,
    status: []const u8,
    usage_pct: f64,
};

/// A single row in the VM disks table.
pub const VmDiskRow = struct {
    vm_name: []const u8,
    vmid: []const u8,
    pool: []const u8,
    size_str: []const u8,
    size_bytes: i64,
};

/// Format bytes into a human-readable string (e.g., "42.1 GiB").
pub fn formatBytes(alloc: Allocator, bytes: i64) []const u8 {
    const fb: f64 = @floatFromInt(@max(bytes, 0));
    if (fb >= 1024.0 * 1024.0 * 1024.0 * 1024.0) {
        return std.fmt.allocPrint(alloc, "{d:.1} TiB", .{fb / (1024.0 * 1024.0 * 1024.0 * 1024.0)}) catch "? TiB";
    } else if (fb >= 1024.0 * 1024.0 * 1024.0) {
        return std.fmt.allocPrint(alloc, "{d:.1} GiB", .{fb / (1024.0 * 1024.0 * 1024.0)}) catch "? GiB";
    } else if (fb >= 1024.0 * 1024.0) {
        return std.fmt.allocPrint(alloc, "{d:.1} MiB", .{fb / (1024.0 * 1024.0)}) catch "? MiB";
    } else {
        return std.fmt.allocPrint(alloc, "{d:.0} KiB", .{fb / 1024.0}) catch "? KiB";
    }
}

/// Thread-safe shared state for storage view data.
pub const StorageState = struct {
    mutex: std.Thread.Mutex = .{},
    pools: []StoragePoolRow = &.{},
    vm_disks: []VmDiskRow = &.{},
    is_loading: bool = true,
    last_refresh_epoch: i64 = 0,
    allocator: Allocator,

    pub fn init(allocator: Allocator) StorageState {
        return .{ .allocator = allocator };
    }

    pub fn swapData(self: *StorageState, new_pools: []StoragePoolRow, new_disks: []VmDiskRow) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.freeDataInternal();
        self.pools = new_pools;
        self.vm_disks = new_disks;
        self.is_loading = false;
        self.last_refresh_epoch = std.time.timestamp();
    }

    pub fn lock(self: *StorageState) void {
        self.mutex.lock();
    }

    pub fn unlock(self: *StorageState) void {
        self.mutex.unlock();
    }

    pub fn isLoading(self: *StorageState) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.is_loading;
    }

    fn freeDataInternal(self: *StorageState) void {
        for (self.pools) |row| {
            self.allocator.free(row.name);
            self.allocator.free(row.node);
            self.allocator.free(row.pool_type);
            self.allocator.free(row.used_str);
            self.allocator.free(row.total_str);
            self.allocator.free(row.status);
        }
        if (self.pools.len > 0) self.allocator.free(self.pools);
        for (self.vm_disks) |row| {
            self.allocator.free(row.vm_name);
            self.allocator.free(row.vmid);
            self.allocator.free(row.pool);
            self.allocator.free(row.size_str);
        }
        if (self.vm_disks.len > 0) self.allocator.free(self.vm_disks);
    }

    pub fn deinit(self: *StorageState) void {
        self.freeDataInternal();
    }
};

/// A single row in the backups table.
pub const BackupRow = struct {
    volid: []const u8,
    node: []const u8,
    storage: []const u8,
    vm_name: []const u8,
    vmid: []const u8,
    size_str: []const u8,
    date_str: []const u8,
    age_days: u32,
    is_stale: bool,
};

/// A single K8s backup row (VolSync/Velero).
pub const K8sBackupRow = struct {
    name: []const u8,
    namespace: []const u8,
    source_type: []const u8,
    status: []const u8,
    schedule: []const u8,
    last_run: []const u8,
};

/// Thread-safe shared state for backup view data.
pub const BackupState = struct {
    mutex: std.Thread.Mutex = .{},
    backups: []BackupRow = &.{},
    k8s_backups: []K8sBackupRow = &.{},
    is_loading: bool = true,
    last_refresh_epoch: i64 = 0,
    allocator: Allocator,

    pub fn init(allocator: Allocator) BackupState {
        return .{ .allocator = allocator };
    }

    pub fn swapData(self: *BackupState, new_backups: []BackupRow, new_k8s: []K8sBackupRow) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.freeDataInternal();
        self.backups = new_backups;
        self.k8s_backups = new_k8s;
        self.is_loading = false;
        self.last_refresh_epoch = std.time.timestamp();
    }

    pub fn lock(self: *BackupState) void {
        self.mutex.lock();
    }

    pub fn unlock(self: *BackupState) void {
        self.mutex.unlock();
    }

    pub fn isLoading(self: *BackupState) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.is_loading;
    }

    fn freeDataInternal(self: *BackupState) void {
        for (self.backups) |row| {
            self.allocator.free(row.volid);
            self.allocator.free(row.node);
            self.allocator.free(row.storage);
            self.allocator.free(row.vm_name);
            self.allocator.free(row.vmid);
            self.allocator.free(row.size_str);
            self.allocator.free(row.date_str);
        }
        if (self.backups.len > 0) self.allocator.free(self.backups);
        for (self.k8s_backups) |row| {
            self.allocator.free(row.name);
            self.allocator.free(row.namespace);
            self.allocator.free(row.source_type);
            self.allocator.free(row.status);
            self.allocator.free(row.schedule);
            self.allocator.free(row.last_run);
        }
        if (self.k8s_backups.len > 0) self.allocator.free(self.k8s_backups);
    }

    pub fn deinit(self: *BackupState) void {
        self.freeDataInternal();
    }
};

/// A single row in the host overview (PVE node metrics).
pub const HostRow = struct {
    name: []const u8,
    cpu_pct: f64, // 0-100
    mem_used_str: []const u8,
    mem_total_str: []const u8,
    mem_pct: f64, // 0-100
};

/// A single row in the pod metrics table.
pub const PodMetricRow = struct {
    pod: []const u8,
    namespace: []const u8,
    cpu_str: []const u8, // e.g. "0.125"
    mem_str: []const u8, // e.g. "128.5 MiB"
    net_rx_str: []const u8, // e.g. "1.2 KiB/s"
    net_tx_str: []const u8, // e.g. "0.5 KiB/s"
    cpu_cores: f64, // for sorting
    mem_bytes: f64, // for sorting
};

/// Thread-safe shared state for performance view data.
pub const PerfState = struct {
    mutex: std.Thread.Mutex = .{},
    hosts: []HostRow = &.{},
    pods: []PodMetricRow = &.{},
    metrics_available: bool = false,
    is_loading: bool = true,
    last_refresh_epoch: i64 = 0,
    allocator: Allocator,

    pub fn init(allocator: Allocator) PerfState {
        return .{ .allocator = allocator };
    }

    pub fn swapData(self: *PerfState, new_hosts: []HostRow, new_pods: []PodMetricRow, available: bool) void {
        self.mutex.lock();
        defer self.mutex.unlock();
        self.freeDataInternal();
        self.hosts = new_hosts;
        self.pods = new_pods;
        self.metrics_available = available;
        self.is_loading = false;
        self.last_refresh_epoch = std.time.timestamp();
    }

    pub fn lock(self: *PerfState) void {
        self.mutex.lock();
    }

    pub fn unlock(self: *PerfState) void {
        self.mutex.unlock();
    }

    pub fn isMetricsAvailable(self: *PerfState) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.metrics_available;
    }

    pub fn isLoading(self: *PerfState) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.is_loading;
    }

    fn freeDataInternal(self: *PerfState) void {
        for (self.hosts) |row| {
            self.allocator.free(row.name);
            self.allocator.free(row.mem_used_str);
            self.allocator.free(row.mem_total_str);
        }
        if (self.hosts.len > 0) self.allocator.free(self.hosts);
        for (self.pods) |row| {
            self.allocator.free(row.pod);
            self.allocator.free(row.namespace);
            self.allocator.free(row.cpu_str);
            self.allocator.free(row.mem_str);
            self.allocator.free(row.net_rx_str);
            self.allocator.free(row.net_tx_str);
        }
        if (self.pods.len > 0) self.allocator.free(self.pods);
    }

    pub fn deinit(self: *PerfState) void {
        self.freeDataInternal();
    }
};

/// Format a rate in bytes/sec into a human-readable string.
pub fn formatRate(alloc: Allocator, bytes_per_sec: f64) []const u8 {
    if (bytes_per_sec >= 1024.0 * 1024.0) {
        return std.fmt.allocPrint(alloc, "{d:.1} MiB/s", .{bytes_per_sec / (1024.0 * 1024.0)}) catch "? MiB/s";
    } else if (bytes_per_sec >= 1024.0) {
        return std.fmt.allocPrint(alloc, "{d:.1} KiB/s", .{bytes_per_sec / 1024.0}) catch "? KiB/s";
    } else {
        return std.fmt.allocPrint(alloc, "{d:.0} B/s", .{bytes_per_sec}) catch "? B/s";
    }
}

/// Format an epoch timestamp into "YYYY-MM-DD HH:MM".
pub fn formatEpoch(alloc: Allocator, epoch: i64) []const u8 {
    const es = std.time.epoch.EpochSeconds{ .secs = @intCast(@max(0, epoch)) };
    const day = es.getEpochDay();
    const yd = day.calculateYearDay();
    const md = yd.calculateMonthDay();
    const ds = es.getDaySeconds();
    return std.fmt.allocPrint(alloc, "{d:0>4}-{d:0>2}-{d:0>2} {d:0>2}:{d:0>2}", .{
        yd.year,
        md.month.numeric(),
        md.day_index + 1,
        ds.getHoursIntoDay(),
        ds.getMinutesIntoHour(),
    }) catch "unknown";
}

/// Thread-safe shared state for cluster view data.
pub const ClusterState = struct {
    mutex: std.Thread.Mutex = .{},
    rows: []NodeRow = &.{},
    is_loading: bool = true,
    error_msg: ?[]const u8 = null,
    last_refresh_epoch: i64 = 0,
    allocator: Allocator,

    pub fn init(allocator: Allocator) ClusterState {
        return .{ .allocator = allocator };
    }

    /// Replace current rows with new data. Frees old rows under mutex.
    pub fn swapRows(self: *ClusterState, new_rows: []NodeRow) void {
        self.mutex.lock();
        defer self.mutex.unlock();

        self.freeRowsInternal();
        self.rows = new_rows;
        self.is_loading = false;
        self.last_refresh_epoch = std.time.timestamp();
    }

    pub fn lock(self: *ClusterState) void {
        self.mutex.lock();
    }

    pub fn unlock(self: *ClusterState) void {
        self.mutex.unlock();
    }

    pub fn getLastRefresh(self: *ClusterState) i64 {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.last_refresh_epoch;
    }

    pub fn isLoading(self: *ClusterState) bool {
        self.mutex.lock();
        defer self.mutex.unlock();
        return self.is_loading;
    }

    fn freeRowsInternal(self: *ClusterState) void {
        for (self.rows) |row| {
            self.allocator.free(row.name);
            self.allocator.free(row.role);
            self.allocator.free(row.ip);
            self.allocator.free(row.pve_node);
            self.allocator.free(row.vmid);
            self.allocator.free(row.talos_ver);
            self.allocator.free(row.k8s_ver);
            self.allocator.free(row.etcd);
            self.allocator.free(row.health);
        }
        if (self.rows.len > 0) {
            self.allocator.free(self.rows);
        }
    }

    pub fn deinit(self: *ClusterState) void {
        self.freeRowsInternal();
    }
};

/// Background poller that fetches data from Proxmox and Talos APIs.
pub const Poller = struct {
    state: *ClusterState,
    storage_state: *StorageState,
    backup_state: *BackupState,
    perf_state: *PerfState,
    cfg: *const config.Config,
    interval_ns: u64,
    should_stop: std.atomic.Value(bool) = std.atomic.Value(bool).init(false),
    force_refresh: std.atomic.Value(bool) = std.atomic.Value(bool).init(false),
    thread: ?std.Thread = null,
    allocator: Allocator,

    pub fn init(
        allocator: Allocator,
        state: *ClusterState,
        storage_state: *StorageState,
        backup_state: *BackupState,
        perf_state: *PerfState,
        cfg: *const config.Config,
        interval_ms: u64,
    ) Poller {
        return .{
            .state = state,
            .storage_state = storage_state,
            .backup_state = backup_state,
            .perf_state = perf_state,
            .cfg = cfg,
            .interval_ns = interval_ms * std.time.ns_per_ms,
            .allocator = allocator,
        };
    }

    pub fn start(self: *Poller) !void {
        self.thread = try std.Thread.spawn(.{}, pollLoop, .{self});
    }

    pub fn stop(self: *Poller) void {
        self.should_stop.store(true, .release);
        if (self.thread) |t| {
            t.join();
            self.thread = null;
        }
    }

    pub fn triggerRefresh(self: *Poller) void {
        self.force_refresh.store(true, .release);
    }

    fn pollLoop(self: *Poller) void {
        while (!self.should_stop.load(.acquire)) {
            self.fetchAll();

            // Sleep in small increments to allow responsive stopping/force-refresh
            var slept: u64 = 0;
            const step = 500 * std.time.ns_per_ms; // 500ms increments
            while (slept < self.interval_ns) {
                if (self.should_stop.load(.acquire)) return;
                if (self.force_refresh.load(.acquire)) {
                    self.force_refresh.store(false, .release);
                    break;
                }
                std.Thread.sleep(step);
                slept += step;
            }
        }
    }

    fn fetchAll(self: *Poller) void {
        const alloc = self.allocator;

        // Collect all configured nodes
        var rows_list: std.ArrayListUnmanaged(NodeRow) = .empty;

        for (self.cfg.clusters) |cluster| {
            // Find the matching PVE cluster config
            var pve_cluster: ?config.ProxmoxCluster = null;
            for (self.cfg.proxmox.clusters) |pc| {
                if (std.mem.eql(u8, pc.name, cluster.proxmox_cluster)) {
                    pve_cluster = pc;
                    break;
                }
            }

            // Fetch PVE VM statuses
            var pve_vms: []proxmox.VmStatus = &.{};
            if (pve_cluster) |pc| {
                var pve_client = proxmox.ProxmoxClient.init(alloc, pc);
                defer pve_client.deinit();
                pve_vms = pve_client.getClusterResources() catch &.{};
            }
            defer {
                for (pve_vms) |vm| {
                    alloc.free(vm.name);
                    alloc.free(vm.status);
                    alloc.free(vm.node);
                }
                if (pve_vms.len > 0) alloc.free(pve_vms);
            }

            // Fetch Talos etcd members
            var talos_client = talos.TalosClient.init(alloc, self.cfg.talos);
            defer talos_client.deinit();
            const etcd_members = talos_client.getEtcdMembers();
            defer {
                for (etcd_members) |m| alloc.free(m.hostname);
                if (etcd_members.len > 0) alloc.free(etcd_members);
            }

            // Build a row for each configured node
            for (cluster.nodes) |node| {
                // Match PVE VM status by VMID
                var vm_status: []const u8 = "unknown";
                for (pve_vms) |vm| {
                    if (vm.vmid == node.proxmox_vmid) {
                        vm_status = vm.status;
                        break;
                    }
                }

                // Match etcd membership by hostname
                var etcd_role: []const u8 = "-";
                for (etcd_members) |m| {
                    if (std.mem.eql(u8, m.hostname, node.name)) {
                        etcd_role = if (m.is_learner) "learner" else "member";
                        break;
                    }
                }

                // Fetch Talos version for this node
                var talos_ver: []const u8 = "-";
                var k8s_ver: []const u8 = "-";
                if (talos_client.getVersion(node.ip)) |ver| {
                    talos_ver = ver.talos_version;
                    k8s_ver = ver.kubernetes_version;
                    // Note: ver.node is freed by us since we'll dupe the strings below
                    alloc.free(ver.node);
                }

                // Determine health
                const health: []const u8 = if (std.mem.eql(u8, vm_status, "running"))
                    (if (!std.mem.eql(u8, talos_ver, "-")) "healthy" else "degraded")
                else if (std.mem.eql(u8, vm_status, "stopped"))
                    "stopped"
                else
                    "unknown";

                const vmid_str = std.fmt.allocPrint(alloc, "{d}", .{node.proxmox_vmid}) catch continue;

                rows_list.append(alloc, .{
                    .name = alloc.dupe(u8, node.name) catch continue,
                    .role = alloc.dupe(u8, node.role) catch continue,
                    .ip = alloc.dupe(u8, node.ip) catch continue,
                    .pve_node = alloc.dupe(u8, node.proxmox_node) catch continue,
                    .vmid = vmid_str,
                    .talos_ver = alloc.dupe(u8, talos_ver) catch continue,
                    .k8s_ver = alloc.dupe(u8, k8s_ver) catch continue,
                    .etcd = alloc.dupe(u8, etcd_role) catch continue,
                    .health = alloc.dupe(u8, health) catch continue,
                }) catch continue;

                // Free talos version strings if they were allocated
                if (!std.mem.eql(u8, talos_ver, "-")) alloc.free(talos_ver);
                if (!std.mem.eql(u8, k8s_ver, "-")) alloc.free(k8s_ver);
            }
        }

        const new_rows = rows_list.toOwnedSlice(alloc) catch return;
        self.state.swapRows(new_rows);

        // Fetch storage data
        self.fetchStorage();

        // Fetch backup data
        self.fetchBackups();

        // Fetch performance data
        self.fetchPerformance();
    }

    fn fetchStorage(self: *Poller) void {
        const alloc = self.allocator;
        var pools_list: std.ArrayListUnmanaged(StoragePoolRow) = .empty;
        var disks_list: std.ArrayListUnmanaged(VmDiskRow) = .empty;

        for (self.cfg.proxmox.clusters) |pc| {
            var pve_client = proxmox.ProxmoxClient.init(alloc, pc);
            defer pve_client.deinit();

            // Fetch storage pools
            const storage_pools = pve_client.getStoragePools() catch &.{};
            defer {
                for (storage_pools) |sp| {
                    alloc.free(sp.name);
                    alloc.free(sp.node);
                    alloc.free(sp.pool_type);
                    alloc.free(sp.status);
                }
                if (storage_pools.len > 0) alloc.free(storage_pools);
            }

            for (storage_pools) |sp| {
                const pct: f64 = if (sp.maxdisk > 0)
                    @as(f64, @floatFromInt(sp.disk)) / @as(f64, @floatFromInt(sp.maxdisk)) * 100.0
                else
                    0.0;

                pools_list.append(alloc, .{
                    .name = alloc.dupe(u8, sp.name) catch continue,
                    .node = alloc.dupe(u8, sp.node) catch continue,
                    .pool_type = alloc.dupe(u8, sp.pool_type) catch continue,
                    .used_str = formatBytes(alloc, sp.disk),
                    .total_str = formatBytes(alloc, sp.maxdisk),
                    .status = alloc.dupe(u8, sp.status) catch continue,
                    .usage_pct = pct,
                }) catch continue;
            }

            // Fetch VMs for disk info
            const vms = pve_client.getClusterResources() catch &.{};
            defer {
                for (vms) |vm| {
                    alloc.free(vm.name);
                    alloc.free(vm.status);
                    alloc.free(vm.node);
                }
                if (vms.len > 0) alloc.free(vms);
            }

            for (vms) |vm| {
                disks_list.append(alloc, .{
                    .vm_name = alloc.dupe(u8, vm.name) catch continue,
                    .vmid = std.fmt.allocPrint(alloc, "{d}", .{vm.vmid}) catch continue,
                    .pool = alloc.dupe(u8, vm.node) catch continue,
                    .size_str = formatBytes(alloc, vm.maxdisk),
                    .size_bytes = vm.maxdisk,
                }) catch continue;
            }
        }

        const new_pools = pools_list.toOwnedSlice(alloc) catch return;
        const new_disks = disks_list.toOwnedSlice(alloc) catch return;
        self.storage_state.swapData(new_pools, new_disks);
    }

    fn fetchBackups(self: *Poller) void {
        const alloc = self.allocator;
        var backups_list: std.ArrayListUnmanaged(BackupRow) = .empty;

        for (self.cfg.proxmox.clusters) |pc| {
            var pve_client = proxmox.ProxmoxClient.init(alloc, pc);
            defer pve_client.deinit();

            // Get storage pools to know where to look for backups
            const pools = pve_client.getStoragePools() catch &.{};
            defer {
                for (pools) |sp| {
                    alloc.free(sp.name);
                    alloc.free(sp.node);
                    alloc.free(sp.pool_type);
                    alloc.free(sp.status);
                }
                if (pools.len > 0) alloc.free(pools);
            }

            // Get VMs for name lookup
            const vms = pve_client.getClusterResources() catch &.{};
            defer {
                for (vms) |vm| {
                    alloc.free(vm.name);
                    alloc.free(vm.status);
                    alloc.free(vm.node);
                }
                if (vms.len > 0) alloc.free(vms);
            }

            // For each storage pool, list backups
            for (pools) |sp| {
                const entries = pve_client.listBackups(sp.node, sp.name) catch &.{};
                defer {
                    for (entries) |e| {
                        alloc.free(e.volid);
                        alloc.free(e.node);
                        alloc.free(e.storage);
                        alloc.free(e.format);
                    }
                    if (entries.len > 0) alloc.free(entries);
                }

                for (entries) |entry| {
                    // Find VM name by VMID
                    var vm_name: []const u8 = "unknown";
                    for (vms) |vm| {
                        if (vm.vmid == entry.vmid) {
                            vm_name = vm.name;
                            break;
                        }
                    }

                    // Compute age
                    const now = std.time.timestamp();
                    const age_secs = now - entry.ctime;
                    const age_days: u32 = @intCast(@max(0, @divTrunc(age_secs, 86400)));
                    const is_stale = age_days > self.cfg.tui_settings.stale_days;

                    backups_list.append(alloc, .{
                        .volid = alloc.dupe(u8, entry.volid) catch continue,
                        .node = alloc.dupe(u8, entry.node) catch continue,
                        .storage = alloc.dupe(u8, entry.storage) catch continue,
                        .vm_name = alloc.dupe(u8, vm_name) catch continue,
                        .vmid = std.fmt.allocPrint(alloc, "{d}", .{entry.vmid}) catch continue,
                        .size_str = formatBytes(alloc, entry.size),
                        .date_str = formatEpoch(alloc, entry.ctime),
                        .age_days = age_days,
                        .is_stale = is_stale,
                    }) catch continue;
                }
            }
        }

        const new_backups = backups_list.toOwnedSlice(alloc) catch return;
        const new_k8s = self.fetchK8sBackups();
        self.backup_state.swapData(new_backups, new_k8s);
    }

    fn fetchK8sBackups(self: *Poller) []K8sBackupRow {
        const alloc = self.allocator;
        const kubeconfig = kubernetes.deriveKubeconfig(alloc, self.cfg.talos.config_path) orelse return &.{};
        defer alloc.free(kubeconfig);

        var client = kubernetes.KubeClient.init(alloc, kubeconfig);
        defer client.deinit();

        const providers = client.detectProviders();
        var k8s_list: std.ArrayListUnmanaged(K8sBackupRow) = .empty;

        if (providers.volsync) {
            const entries = client.getVolsyncSources();
            defer {
                for (entries) |e| {
                    alloc.free(e.name);
                    alloc.free(e.namespace);
                    alloc.free(e.source_type);
                    alloc.free(e.status);
                    alloc.free(e.schedule);
                    alloc.free(e.last_run);
                }
                if (entries.len > 0) alloc.free(entries);
            }
            for (entries) |e| {
                k8s_list.append(alloc, .{
                    .name = alloc.dupe(u8, e.name) catch continue,
                    .namespace = alloc.dupe(u8, e.namespace) catch continue,
                    .source_type = alloc.dupe(u8, e.source_type) catch continue,
                    .status = alloc.dupe(u8, e.status) catch continue,
                    .schedule = alloc.dupe(u8, e.schedule) catch continue,
                    .last_run = alloc.dupe(u8, e.last_run) catch continue,
                }) catch continue;
            }
        }

        if (providers.velero) {
            const entries = client.getVeleroBackups();
            defer {
                for (entries) |e| {
                    alloc.free(e.name);
                    alloc.free(e.namespace);
                    alloc.free(e.source_type);
                    alloc.free(e.status);
                    alloc.free(e.schedule);
                    alloc.free(e.last_run);
                }
                if (entries.len > 0) alloc.free(entries);
            }
            for (entries) |e| {
                k8s_list.append(alloc, .{
                    .name = alloc.dupe(u8, e.name) catch continue,
                    .namespace = alloc.dupe(u8, e.namespace) catch continue,
                    .source_type = alloc.dupe(u8, e.source_type) catch continue,
                    .status = alloc.dupe(u8, e.status) catch continue,
                    .schedule = alloc.dupe(u8, e.schedule) catch continue,
                    .last_run = alloc.dupe(u8, e.last_run) catch continue,
                }) catch continue;
            }
        }

        return k8s_list.toOwnedSlice(alloc) catch &.{};
    }

    fn fetchPerformance(self: *Poller) void {
        const alloc = self.allocator;

        // Host metrics from PVE API
        var hosts_list: std.ArrayListUnmanaged(HostRow) = .empty;
        for (self.cfg.proxmox.clusters) |pc| {
            var pve_client = proxmox.ProxmoxClient.init(alloc, pc);
            defer pve_client.deinit();

            // Get distinct node names from cluster resources
            const vms = pve_client.getClusterResources() catch &.{};
            defer {
                for (vms) |vm| {
                    alloc.free(vm.name);
                    alloc.free(vm.status);
                    alloc.free(vm.node);
                }
                if (vms.len > 0) alloc.free(vms);
            }

            // Collect unique node names
            var seen_nodes: std.ArrayListUnmanaged([]const u8) = .empty;
            defer {
                for (seen_nodes.items) |n| alloc.free(n);
                seen_nodes.deinit(alloc);
            }

            for (vms) |vm| {
                var found = false;
                for (seen_nodes.items) |n| {
                    if (std.mem.eql(u8, n, vm.node)) {
                        found = true;
                        break;
                    }
                }
                if (!found) {
                    seen_nodes.append(alloc, alloc.dupe(u8, vm.node) catch continue) catch continue;
                }
            }

            for (seen_nodes.items) |node_name| {
                const ns = pve_client.getNodeStatus(node_name) catch continue orelse continue;
                const mem_pct: f64 = if (ns.maxmem > 0)
                    @as(f64, @floatFromInt(ns.mem)) / @as(f64, @floatFromInt(ns.maxmem)) * 100.0
                else
                    0.0;

                hosts_list.append(alloc, .{
                    .name = alloc.dupe(u8, ns.node) catch continue,
                    .cpu_pct = ns.cpu * 100.0,
                    .mem_used_str = formatBytes(alloc, ns.mem),
                    .mem_total_str = formatBytes(alloc, ns.maxmem),
                    .mem_pct = mem_pct,
                }) catch continue;

                alloc.free(ns.node);
                alloc.free(ns.status);
            }
        }

        // Pod metrics from Prometheus/VictoriaMetrics
        var pods_list: std.ArrayListUnmanaged(PodMetricRow) = .empty;
        var metrics_available = false;

        const kubeconfig = kubernetes.deriveKubeconfig(alloc, self.cfg.talos.config_path);
        if (kubeconfig) |kc| {
            defer alloc.free(kc);

            var mc = metrics_api.MetricsClient.init(alloc, kc);
            defer mc.deinit();

            if (mc.available) {
                metrics_available = true;

                const cpu_data = mc.getPodCpu();
                defer self.freeMetricValues(cpu_data);

                const mem_data = mc.getPodMemory();
                defer self.freeMetricValues(mem_data);

                const rx_data = mc.getPodNetRx();
                defer self.freeMetricValues(rx_data);

                const tx_data = mc.getPodNetTx();
                defer self.freeMetricValues(tx_data);

                // Build pod map from CPU data (most common metric)
                for (cpu_data) |cpu| {
                    const pod_name = getLabelStr(cpu.labels, "pod");
                    const ns_name = getLabelStr(cpu.labels, "namespace");

                    // Find matching memory
                    var mem_val: f64 = 0;
                    for (mem_data) |m| {
                        if (std.mem.eql(u8, getLabelStr(m.labels, "pod"), pod_name) and
                            std.mem.eql(u8, getLabelStr(m.labels, "namespace"), ns_name))
                        {
                            mem_val = m.value;
                            break;
                        }
                    }

                    // Find matching network
                    var rx_val: f64 = 0;
                    var tx_val: f64 = 0;
                    for (rx_data) |r| {
                        if (std.mem.eql(u8, getLabelStr(r.labels, "pod"), pod_name) and
                            std.mem.eql(u8, getLabelStr(r.labels, "namespace"), ns_name))
                        {
                            rx_val = r.value;
                            break;
                        }
                    }
                    for (tx_data) |t| {
                        if (std.mem.eql(u8, getLabelStr(t.labels, "pod"), pod_name) and
                            std.mem.eql(u8, getLabelStr(t.labels, "namespace"), ns_name))
                        {
                            tx_val = t.value;
                            break;
                        }
                    }

                    pods_list.append(alloc, .{
                        .pod = alloc.dupe(u8, pod_name) catch continue,
                        .namespace = alloc.dupe(u8, ns_name) catch continue,
                        .cpu_str = std.fmt.allocPrint(alloc, "{d:.3}", .{cpu.value}) catch continue,
                        .mem_str = formatBytes(alloc, @intFromFloat(@max(0, mem_val))),
                        .net_rx_str = formatRate(alloc, rx_val),
                        .net_tx_str = formatRate(alloc, tx_val),
                        .cpu_cores = cpu.value,
                        .mem_bytes = mem_val,
                    }) catch continue;
                }
            }
        }

        const new_hosts = hosts_list.toOwnedSlice(alloc) catch return;
        const new_pods = pods_list.toOwnedSlice(alloc) catch return;
        self.perf_state.swapData(new_hosts, new_pods, metrics_available);
    }

    fn freeMetricValues(self: *Poller, values: []metrics_api.MetricsClient.PodMetricValue) void {
        for (values) |v| {
            var it = v.labels.iterator();
            while (it.next()) |entry| {
                self.allocator.free(entry.key_ptr.*);
                switch (entry.value_ptr.*) {
                    .string => |s| self.allocator.free(s),
                    else => {},
                }
            }
            var labels_copy = v.labels;
            labels_copy.deinit();
        }
        if (values.len > 0) self.allocator.free(values);
    }

    pub fn deinit(self: *Poller) void {
        self.stop();
    }
};

fn getLabelStr(labels: std.json.ObjectMap, key: []const u8) []const u8 {
    const val = labels.get(key) orelse return "";
    return switch (val) {
        .string => |s| s,
        else => "",
    };
}
