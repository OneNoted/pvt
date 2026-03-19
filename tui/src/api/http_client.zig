const std = @import("std");
const config = @import("../config.zig");
const Allocator = std.mem.Allocator;

/// HTTP client that uses curl subprocess for Proxmox API requests.
/// Handles PVE API token auth and TLS certificate skipping for self-signed certs.
pub const HttpClient = struct {
    allocator: Allocator,
    endpoint: []const u8,
    token_id: []const u8,
    token_secret: []const u8,
    tls_verify: bool,

    pub fn init(allocator: Allocator, pve: config.ProxmoxCluster) HttpClient {
        return .{
            .allocator = allocator,
            .endpoint = pve.endpoint,
            .token_id = pve.token_id,
            .token_secret = pve.token_secret,
            .tls_verify = pve.tls_verify,
        };
    }

    /// Perform a GET request. Caller owns the returned memory.
    pub fn get(self: HttpClient, path: []const u8) ![]const u8 {
        return self.request("GET", path);
    }

    /// Perform a DELETE request. Caller owns the returned memory.
    pub fn delete(self: HttpClient, path: []const u8) ![]const u8 {
        return self.request("DELETE", path);
    }

    fn request(self: HttpClient, method: []const u8, path: []const u8) ![]const u8 {
        const url = try std.fmt.allocPrint(self.allocator, "{s}{s}", .{ self.endpoint, path });
        defer self.allocator.free(url);

        const auth = try std.fmt.allocPrint(self.allocator, "Authorization: PVEAPIToken={s}={s}", .{ self.token_id, self.token_secret });
        defer self.allocator.free(auth);

        var argv_list: std.ArrayListUnmanaged([]const u8) = .empty;
        defer argv_list.deinit(self.allocator);

        try argv_list.appendSlice(self.allocator, &.{ "curl", "-s", "-f", "--max-time", "10" });
        if (!std.mem.eql(u8, method, "GET")) {
            try argv_list.appendSlice(self.allocator, &.{ "-X", method });
        }
        try argv_list.appendSlice(self.allocator, &.{ "-H", auth });
        if (!self.tls_verify) {
            try argv_list.append(self.allocator, "-k");
        }
        try argv_list.append(self.allocator, url);

        const result = std.process.Child.run(.{
            .allocator = self.allocator,
            .argv = argv_list.items,
            .max_output_bytes = 1024 * 1024,
        }) catch |err| {
            std.log.err("failed to run curl: {}", .{err});
            return error.HttpRequestFailed;
        };
        defer self.allocator.free(result.stderr);

        const term = result.term;
        if (term == .Exited and term.Exited == 0) {
            return result.stdout;
        }

        self.allocator.free(result.stdout);
        std.log.err("curl {s} failed (exit {}): {s}", .{ method, term, result.stderr });
        return error.HttpRequestFailed;
    }

    pub fn deinit(self: *HttpClient) void {
        _ = self;
    }
};

/// Parse a JSON response body and extract the "data" field.
/// Returns the parsed JSON Value. Caller must call `parsed.deinit()`.
pub fn parseJsonResponse(allocator: Allocator, body: []const u8) !std.json.Parsed(std.json.Value) {
    return std.json.parseFromSlice(std.json.Value, allocator, body, .{
        .ignore_unknown_fields = true,
        .allocate = .alloc_always,
    });
}

/// Extract a string field from a JSON object, returning a default if missing.
pub fn jsonStr(obj: std.json.ObjectMap, key: []const u8, default: []const u8) []const u8 {
    const val = obj.get(key) orelse return default;
    return switch (val) {
        .string => |s| s,
        else => default,
    };
}

/// Extract an integer field from a JSON object, returning a default if missing.
pub fn jsonInt(obj: std.json.ObjectMap, key: []const u8, default: i64) i64 {
    const val = obj.get(key) orelse return default;
    return switch (val) {
        .integer => |i| i,
        .float => |f| @intFromFloat(f),
        .string => |s| std.fmt.parseInt(i64, s, 10) catch default,
        else => default,
    };
}

/// Extract a float field from a JSON object, returning a default if missing.
pub fn jsonFloat(obj: std.json.ObjectMap, key: []const u8, default: f64) f64 {
    const val = obj.get(key) orelse return default;
    return switch (val) {
        .float => |f| f,
        .integer => |i| @floatFromInt(i),
        else => default,
    };
}

test "jsonStr returns default for missing key" {
    var map = std.json.ObjectMap.init(std.testing.allocator);
    defer map.deinit();
    try std.testing.expectEqualStrings("fallback", jsonStr(map, "missing", "fallback"));
}
