const std = @import("std");
const config = @import("../config.zig");
const http = @import("http_client.zig");
const Allocator = std.mem.Allocator;

pub const TalosVersion = struct {
    node: []const u8,
    talos_version: []const u8,
    kubernetes_version: []const u8,
};

pub const EtcdMember = struct {
    hostname: []const u8,
    id: u64,
    is_learner: bool,
};

pub const TalosClient = struct {
    allocator: Allocator,
    config_path: []const u8,
    context: []const u8,

    pub fn init(allocator: Allocator, talos_cfg: config.TalosConfig) TalosClient {
        return .{
            .allocator = allocator,
            .config_path = talos_cfg.config_path,
            .context = talos_cfg.context,
        };
    }

    /// Get Talos and Kubernetes version for a specific node.
    /// Returns null if the node is unreachable.
    pub fn getVersion(self: *TalosClient, node_ip: []const u8) ?TalosVersion {
        const output = self.runTalosctl(&.{
            "version", "--nodes", node_ip, "--short",
        }) orelse return null;
        defer self.allocator.free(output);

        // Parse the JSON output. talosctl version -o json outputs messages array.
        var parsed = std.json.parseFromSlice(std.json.Value, self.allocator, output, .{
            .ignore_unknown_fields = true,
            .allocate = .alloc_always,
        }) catch return null;
        defer parsed.deinit();

        // talosctl version -o json structure:
        // {"messages":[{"metadata":{"hostname":"..."},"version":{"tag":"v1.9.x","...":"..."}}]}
        const root = switch (parsed.value) {
            .object => |obj| obj,
            else => return null,
        };

        const messages = switch (root.get("messages") orelse return null) {
            .array => |arr| arr.items,
            else => return null,
        };
        if (messages.len == 0) return null;

        const msg = switch (messages[0]) {
            .object => |obj| obj,
            else => return null,
        };

        // Extract version info
        const version_obj = switch (msg.get("version") orelse return null) {
            .object => |obj| obj,
            else => return null,
        };

        const talos_ver = http.jsonStr(version_obj, "tag", "unknown");
        // Kubernetes version is typically in a separate field or needs a different query
        // For now extract what's available
        const k8s_ver = http.jsonStr(version_obj, "kubernetes_version", "-");

        return .{
            .node = self.allocator.dupe(u8, node_ip) catch return null,
            .talos_version = self.allocator.dupe(u8, talos_ver) catch return null,
            .kubernetes_version = self.allocator.dupe(u8, k8s_ver) catch return null,
        };
    }

    /// Get etcd cluster membership info.
    /// Returns empty slice if unreachable.
    pub fn getEtcdMembers(self: *TalosClient) []EtcdMember {
        const output = self.runTalosctl(&.{"etcd", "members"}) orelse return &.{};
        defer self.allocator.free(output);

        var parsed = std.json.parseFromSlice(std.json.Value, self.allocator, output, .{
            .ignore_unknown_fields = true,
            .allocate = .alloc_always,
        }) catch return &.{};
        defer parsed.deinit();

        const root = switch (parsed.value) {
            .object => |obj| obj,
            else => return &.{},
        };

        const messages = switch (root.get("messages") orelse return &.{}) {
            .array => |arr| arr.items,
            else => return &.{},
        };
        if (messages.len == 0) return &.{};

        const msg = switch (messages[0]) {
            .object => |obj| obj,
            else => return &.{},
        };

        const members = switch (msg.get("members") orelse return &.{}) {
            .array => |arr| arr.items,
            else => return &.{},
        };

        var results: std.ArrayListUnmanaged(EtcdMember) = .empty;
        for (members) |item| {
            const obj = switch (item) {
                .object => |o| o,
                else => continue,
            };
            results.append(self.allocator, .{
                .hostname = self.allocator.dupe(u8, http.jsonStr(obj, "hostname", "unknown")) catch continue,
                .id = @intCast(http.jsonInt(obj, "id", 0)),
                .is_learner = blk: {
                    const val = obj.get("is_learner") orelse break :blk false;
                    break :blk switch (val) {
                        .bool => |b| b,
                        else => false,
                    };
                },
            }) catch continue;
        }

        return results.toOwnedSlice(self.allocator) catch &.{};
    }

    /// Run a talosctl command with standard flags and return stdout.
    /// Returns null on any failure.
    fn runTalosctl(self: *TalosClient, extra_args: []const []const u8) ?[]const u8 {
        var argv: std.ArrayListUnmanaged([]const u8) = .empty;
        defer argv.deinit(self.allocator);

        argv.append(self.allocator, "talosctl") catch return null;
        argv.appendSlice(self.allocator, extra_args) catch return null;
        argv.appendSlice(self.allocator, &.{
            "--talosconfig", self.config_path,
            "--context",     self.context,
            "-o",            "json",
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

    pub fn deinit(self: *TalosClient) void {
        _ = self;
    }
};
