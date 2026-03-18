const std = @import("std");
const config = @import("../config.zig");
const http = @import("http_client.zig");
const Allocator = std.mem.Allocator;

pub const VmStatus = struct {
    vmid: i64,
    name: []const u8,
    status: []const u8,
    node: []const u8,
    maxdisk: i64,
};

pub const StoragePool = struct {
    name: []const u8,
    node: []const u8,
    pool_type: []const u8,
    status: []const u8,
    disk: i64,
    maxdisk: i64,
};

pub const NodeStatus = struct {
    node: []const u8,
    status: []const u8,
    cpu: f64,
    mem: i64,
    maxmem: i64,
    uptime: i64,
};

pub const BackupEntry = struct {
    volid: []const u8,
    node: []const u8,
    storage: []const u8,
    size: i64,
    ctime: i64,
    vmid: i64,
    format: []const u8,
};

pub const ProxmoxClient = struct {
    client: http.HttpClient,
    allocator: Allocator,

    pub fn init(allocator: Allocator, pve: config.ProxmoxCluster) ProxmoxClient {
        return .{
            .client = http.HttpClient.init(allocator, pve),
            .allocator = allocator,
        };
    }

    /// Fetch all VM resources across the PVE cluster.
    pub fn getClusterResources(self: *ProxmoxClient) ![]VmStatus {
        const body = self.client.get("/api2/json/cluster/resources?type=vm") catch {
            return &.{};
        };
        defer self.allocator.free(body);

        var parsed = http.parseJsonResponse(self.allocator, body) catch {
            return &.{};
        };
        defer parsed.deinit();

        const data_val = switch (parsed.value) {
            .object => |obj| obj.get("data") orelse return &.{},
            else => return &.{},
        };
        const items = switch (data_val) {
            .array => |arr| arr.items,
            else => return &.{},
        };

        var results: std.ArrayListUnmanaged(VmStatus) = .empty;
        for (items) |item| {
            const obj = switch (item) {
                .object => |o| o,
                else => continue,
            };

            // Only include QEMU VMs (not LXC containers)
            const res_type = http.jsonStr(obj, "type", "");
            if (!std.mem.eql(u8, res_type, "qemu")) continue;

            const name = try self.allocator.dupe(u8, http.jsonStr(obj, "name", "unknown"));
            const status = try self.allocator.dupe(u8, http.jsonStr(obj, "status", "unknown"));
            const node = try self.allocator.dupe(u8, http.jsonStr(obj, "node", "unknown"));

            try results.append(self.allocator, .{
                .vmid = http.jsonInt(obj, "vmid", 0),
                .name = name,
                .status = status,
                .node = node,
                .maxdisk = http.jsonInt(obj, "maxdisk", 0),
            });
        }

        return results.toOwnedSlice(self.allocator);
    }

    /// Fetch status for a specific PVE node.
    pub fn getNodeStatus(self: *ProxmoxClient, node: []const u8) !?NodeStatus {
        const path = try std.fmt.allocPrint(self.allocator, "/api2/json/nodes/{s}/status", .{node});
        defer self.allocator.free(path);

        const body = self.client.get(path) catch return null;
        defer self.allocator.free(body);

        var parsed = http.parseJsonResponse(self.allocator, body) catch return null;
        defer parsed.deinit();

        const data_val = switch (parsed.value) {
            .object => |obj| obj.get("data") orelse return null,
            else => return null,
        };
        const obj = switch (data_val) {
            .object => |o| o,
            else => return null,
        };

        return .{
            .node = try self.allocator.dupe(u8, node),
            .status = try self.allocator.dupe(u8, http.jsonStr(obj, "status", "unknown")),
            .cpu = http.jsonFloat(obj, "cpu", 0),
            .mem = http.jsonInt(obj, "mem", 0),
            .maxmem = http.jsonInt(obj, "maxmem", 0),
            .uptime = http.jsonInt(obj, "uptime", 0),
        };
    }

    /// Fetch all storage pools across the PVE cluster.
    pub fn getStoragePools(self: *ProxmoxClient) ![]StoragePool {
        const body = self.client.get("/api2/json/cluster/resources?type=storage") catch {
            return &.{};
        };
        defer self.allocator.free(body);

        var parsed = http.parseJsonResponse(self.allocator, body) catch {
            return &.{};
        };
        defer parsed.deinit();

        const data_val = switch (parsed.value) {
            .object => |obj| obj.get("data") orelse return &.{},
            else => return &.{},
        };
        const items = switch (data_val) {
            .array => |arr| arr.items,
            else => return &.{},
        };

        var results: std.ArrayListUnmanaged(StoragePool) = .empty;
        for (items) |item| {
            const obj = switch (item) {
                .object => |o| o,
                else => continue,
            };

            const name = try self.allocator.dupe(u8, http.jsonStr(obj, "storage", "unknown"));
            const node = try self.allocator.dupe(u8, http.jsonStr(obj, "node", "unknown"));
            const pool_type = try self.allocator.dupe(u8, http.jsonStr(obj, "plugintype", http.jsonStr(obj, "type", "unknown")));
            const status = try self.allocator.dupe(u8, http.jsonStr(obj, "status", "unknown"));

            try results.append(self.allocator, .{
                .name = name,
                .node = node,
                .pool_type = pool_type,
                .status = status,
                .disk = http.jsonInt(obj, "disk", 0),
                .maxdisk = http.jsonInt(obj, "maxdisk", 0),
            });
        }

        return results.toOwnedSlice(self.allocator);
    }

    /// List vzdump backups from a specific storage pool on a node.
    pub fn listBackups(self: *ProxmoxClient, node: []const u8, storage: []const u8) ![]BackupEntry {
        const path = try std.fmt.allocPrint(self.allocator, "/api2/json/nodes/{s}/storage/{s}/content?content=backup", .{ node, storage });
        defer self.allocator.free(path);

        const body = self.client.get(path) catch return &.{};
        defer self.allocator.free(body);

        var parsed = http.parseJsonResponse(self.allocator, body) catch return &.{};
        defer parsed.deinit();

        const data_val = switch (parsed.value) {
            .object => |obj| obj.get("data") orelse return &.{},
            else => return &.{},
        };
        const items = switch (data_val) {
            .array => |arr| arr.items,
            else => return &.{},
        };

        var results: std.ArrayListUnmanaged(BackupEntry) = .empty;
        for (items) |item| {
            const obj = switch (item) {
                .object => |o| o,
                else => continue,
            };

            const volid = try self.allocator.dupe(u8, http.jsonStr(obj, "volid", ""));
            const format = try self.allocator.dupe(u8, http.jsonStr(obj, "format", "unknown"));

            try results.append(self.allocator, .{
                .volid = volid,
                .node = try self.allocator.dupe(u8, node),
                .storage = try self.allocator.dupe(u8, storage),
                .size = http.jsonInt(obj, "size", 0),
                .ctime = http.jsonInt(obj, "ctime", 0),
                .vmid = http.jsonInt(obj, "vmid", 0),
                .format = format,
            });
        }

        return results.toOwnedSlice(self.allocator);
    }

    /// Delete a backup by volume ID.
    pub fn deleteBackup(self: *ProxmoxClient, node: []const u8, storage: []const u8, volid: []const u8) !void {
        // URL-encode the volid (colons → %3A)
        var encoded: std.ArrayListUnmanaged(u8) = .empty;
        defer encoded.deinit(self.allocator);
        for (volid) |c| {
            if (c == ':') {
                try encoded.appendSlice(self.allocator, "%3A");
            } else {
                try encoded.append(self.allocator, c);
            }
        }

        const path = try std.fmt.allocPrint(self.allocator, "/api2/json/nodes/{s}/storage/{s}/content/{s}", .{
            node, storage, encoded.items,
        });
        defer self.allocator.free(path);

        const body = try self.client.delete(path);
        self.allocator.free(body);
    }

    pub fn deinit(self: *ProxmoxClient) void {
        self.client.deinit();
    }
};
