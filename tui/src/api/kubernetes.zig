const std = @import("std");
const config = @import("../config.zig");
const http = @import("http_client.zig");
const Allocator = std.mem.Allocator;

pub const K8sBackupEntry = struct {
    name: []const u8,
    namespace: []const u8,
    source_type: []const u8, // "VolSync" or "Velero"
    status: []const u8,
    schedule: []const u8,
    last_run: []const u8,
};

pub const DetectedProviders = struct {
    volsync: bool = false,
    velero: bool = false,
};

pub const KubeClient = struct {
    allocator: Allocator,
    kubeconfig: []const u8,

    pub fn init(allocator: Allocator, kubeconfig: []const u8) KubeClient {
        return .{
            .allocator = allocator,
            .kubeconfig = kubeconfig,
        };
    }

    /// Detect which backup providers (VolSync, Velero) are installed by checking CRDs.
    pub fn detectProviders(self: *KubeClient) DetectedProviders {
        const output = self.runKubectl(&.{
            "get", "crd", "--no-headers", "-o", "custom-columns=NAME:.metadata.name",
        }) orelse return .{};
        defer self.allocator.free(output);

        var result = DetectedProviders{};
        var lines = std.mem.splitScalar(u8, output, '\n');
        while (lines.next()) |line| {
            const trimmed = std.mem.trim(u8, line, " \t\r");
            if (trimmed.len == 0) continue;
            if (std.mem.indexOf(u8, trimmed, "volsync") != null) result.volsync = true;
            if (std.mem.indexOf(u8, trimmed, "velero") != null) result.velero = true;
        }
        return result;
    }

    /// Fetch VolSync ReplicationSources across all namespaces.
    pub fn getVolsyncSources(self: *KubeClient) []K8sBackupEntry {
        const output = self.runKubectl(&.{
            "get", "replicationsources.volsync.backube", "-A", "-o", "json",
        }) orelse return &.{};
        defer self.allocator.free(output);

        return self.parseK8sBackups(output, "VolSync");
    }

    /// Fetch Velero Backups across all namespaces.
    pub fn getVeleroBackups(self: *KubeClient) []K8sBackupEntry {
        const output = self.runKubectl(&.{
            "get", "backups.velero.io", "-A", "-o", "json",
        }) orelse return &.{};
        defer self.allocator.free(output);

        return self.parseK8sBackups(output, "Velero");
    }

    fn parseK8sBackups(self: *KubeClient, output: []const u8, source_type: []const u8) []K8sBackupEntry {
        var parsed = std.json.parseFromSlice(std.json.Value, self.allocator, output, .{
            .ignore_unknown_fields = true,
            .allocate = .alloc_always,
        }) catch return &.{};
        defer parsed.deinit();

        const root = switch (parsed.value) {
            .object => |obj| obj,
            else => return &.{},
        };

        const items = switch (root.get("items") orelse return &.{}) {
            .array => |arr| arr.items,
            else => return &.{},
        };

        var results: std.ArrayListUnmanaged(K8sBackupEntry) = .empty;
        for (items) |item| {
            const obj = switch (item) {
                .object => |o| o,
                else => continue,
            };

            const metadata = switch (obj.get("metadata") orelse continue) {
                .object => |o| o,
                else => continue,
            };

            const name = self.allocator.dupe(u8, http.jsonStr(metadata, "name", "unknown")) catch continue;
            const namespace = self.allocator.dupe(u8, http.jsonStr(metadata, "namespace", "default")) catch continue;

            // Extract status and schedule based on source type
            var status: []const u8 = undefined;
            var schedule: []const u8 = undefined;
            var last_run: []const u8 = undefined;

            if (std.mem.eql(u8, source_type, "VolSync")) {
                status = self.parseVolsyncStatus(obj);
                schedule = self.parseVolsyncSchedule(obj);
                last_run = self.parseVolsyncLastRun(obj);
            } else {
                status = self.parseVeleroStatus(obj);
                schedule = self.parseVeleroSchedule(obj);
                last_run = self.parseVeleroLastRun(obj);
            }

            results.append(self.allocator, .{
                .name = name,
                .namespace = namespace,
                .source_type = self.allocator.dupe(u8, source_type) catch continue,
                .status = status,
                .schedule = schedule,
                .last_run = last_run,
            }) catch continue;
        }

        return results.toOwnedSlice(self.allocator) catch &.{};
    }

    fn parseVolsyncStatus(self: *KubeClient, obj: std.json.ObjectMap) []const u8 {
        const status_obj = switch (obj.get("status") orelse return self.allocator.dupe(u8, "unknown") catch "unknown") {
            .object => |o| o,
            else => return self.allocator.dupe(u8, "unknown") catch "unknown",
        };

        // Check conditions array for Synchronizing condition
        const conditions = switch (status_obj.get("conditions") orelse return self.allocator.dupe(u8, "unknown") catch "unknown") {
            .array => |arr| arr.items,
            else => return self.allocator.dupe(u8, "unknown") catch "unknown",
        };

        for (conditions) |cond| {
            const cond_obj = switch (cond) {
                .object => |o| o,
                else => continue,
            };
            const cond_type = http.jsonStr(cond_obj, "type", "");
            if (std.mem.eql(u8, cond_type, "Synchronizing")) {
                const cond_status = http.jsonStr(cond_obj, "status", "Unknown");
                if (std.mem.eql(u8, cond_status, "True")) {
                    return self.allocator.dupe(u8, "Syncing") catch "Syncing";
                }
                return self.allocator.dupe(u8, "Idle") catch "Idle";
            }
        }

        return self.allocator.dupe(u8, "unknown") catch "unknown";
    }

    fn parseVolsyncSchedule(self: *KubeClient, obj: std.json.ObjectMap) []const u8 {
        const spec = switch (obj.get("spec") orelse return self.allocator.dupe(u8, "-") catch "-") {
            .object => |o| o,
            else => return self.allocator.dupe(u8, "-") catch "-",
        };
        const trigger = switch (spec.get("trigger") orelse return self.allocator.dupe(u8, "-") catch "-") {
            .object => |o| o,
            else => return self.allocator.dupe(u8, "-") catch "-",
        };
        return self.allocator.dupe(u8, http.jsonStr(trigger, "schedule", "-")) catch "-";
    }

    fn parseVolsyncLastRun(self: *KubeClient, obj: std.json.ObjectMap) []const u8 {
        const status_obj = switch (obj.get("status") orelse return self.allocator.dupe(u8, "-") catch "-") {
            .object => |o| o,
            else => return self.allocator.dupe(u8, "-") catch "-",
        };
        return self.allocator.dupe(u8, http.jsonStr(status_obj, "lastSyncTime", "-")) catch "-";
    }

    fn parseVeleroStatus(self: *KubeClient, obj: std.json.ObjectMap) []const u8 {
        const status_obj = switch (obj.get("status") orelse return self.allocator.dupe(u8, "unknown") catch "unknown") {
            .object => |o| o,
            else => return self.allocator.dupe(u8, "unknown") catch "unknown",
        };
        return self.allocator.dupe(u8, http.jsonStr(status_obj, "phase", "unknown")) catch "unknown";
    }

    fn parseVeleroSchedule(self: *KubeClient, obj: std.json.ObjectMap) []const u8 {
        const spec = switch (obj.get("spec") orelse return self.allocator.dupe(u8, "-") catch "-") {
            .object => |o| o,
            else => return self.allocator.dupe(u8, "-") catch "-",
        };
        return self.allocator.dupe(u8, http.jsonStr(spec, "scheduleName", "-")) catch "-";
    }

    fn parseVeleroLastRun(self: *KubeClient, obj: std.json.ObjectMap) []const u8 {
        const status_obj = switch (obj.get("status") orelse return self.allocator.dupe(u8, "-") catch "-") {
            .object => |o| o,
            else => return self.allocator.dupe(u8, "-") catch "-",
        };
        return self.allocator.dupe(u8, http.jsonStr(status_obj, "completionTimestamp", "-")) catch "-";
    }

    /// Run a kubectl command with standard flags and return stdout.
    /// Returns null on any failure.
    fn runKubectl(self: *KubeClient, extra_args: []const []const u8) ?[]const u8 {
        var argv: std.ArrayListUnmanaged([]const u8) = .empty;
        defer argv.deinit(self.allocator);

        argv.append(self.allocator, "kubectl") catch return null;
        argv.appendSlice(self.allocator, extra_args) catch return null;
        argv.appendSlice(self.allocator, &.{
            "--kubeconfig", self.kubeconfig,
        }) catch return null;

        const result = std.process.Child.run(.{
            .allocator = self.allocator,
            .argv = argv.items,
            .max_output_bytes = 512 * 1024,
        }) catch return null;
        defer self.allocator.free(result.stderr);

        const term = result.term;
        if (term == .Exited and term.Exited == 0) {
            return result.stdout;
        }

        self.allocator.free(result.stdout);
        return null;
    }

    pub fn deinit(self: *KubeClient) void {
        _ = self;
    }
};

/// Derive the kubeconfig path from a talos config path.
/// Given "~/talos/apollo/talosconfig", returns "~/talos/apollo/kubeconfig".
pub fn deriveKubeconfig(allocator: Allocator, talos_config_path: []const u8) ?[]const u8 {
    // Find the last path separator
    const dir_end = std.mem.lastIndexOfScalar(u8, talos_config_path, '/') orelse return null;
    return std.fmt.allocPrint(allocator, "{s}/kubeconfig", .{talos_config_path[0..dir_end]}) catch null;
}

test "deriveKubeconfig" {
    const alloc = std.testing.allocator;
    const result = deriveKubeconfig(alloc, "~/talos/apollo/talosconfig") orelse unreachable;
    defer alloc.free(result);
    try std.testing.expectEqualStrings("~/talos/apollo/kubeconfig", result);
}
