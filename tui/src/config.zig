const std = @import("std");
const yaml = @import("yaml");
const Allocator = std.mem.Allocator;

const Value = yaml.Yaml.Value;
const Map = yaml.Yaml.Map;

// ── Config types ─────────────────────────────────────────────────────

pub const Config = struct {
    version: []const u8,
    proxmox: ProxmoxConfig,
    talos: TalosConfig,
    clusters: []const ClusterConfig,
    tui_settings: TuiSettings,
};

pub const ProxmoxConfig = struct {
    clusters: []const ProxmoxCluster,
};

pub const ProxmoxCluster = struct {
    name: []const u8,
    endpoint: []const u8,
    token_id: []const u8,
    token_secret: []const u8,
    tls_verify: bool,
};

pub const TalosConfig = struct {
    config_path: []const u8,
    context: []const u8,
};

pub const ClusterConfig = struct {
    name: []const u8,
    proxmox_cluster: []const u8,
    endpoint: []const u8,
    nodes: []const NodeConfig,
};

pub const NodeConfig = struct {
    name: []const u8,
    role: []const u8,
    proxmox_vmid: i64,
    proxmox_node: []const u8,
    ip: []const u8,
};

pub const TuiSettings = struct {
    warn_threshold: u8 = 10,
    crit_threshold: u8 = 5,
    stale_days: u32 = 30,
    refresh_interval_ms: u64 = 30_000,
};

// ── Parsing helpers ──────────────────────────────────────────────────

fn getStr(map: Map, key: []const u8) ![]const u8 {
    const val = map.get(key) orelse return error.ConfigParseFailed;
    return val.asScalar() orelse return error.ConfigParseFailed;
}

fn getStrOr(map: Map, key: []const u8, default: []const u8) []const u8 {
    const val = map.get(key) orelse return default;
    return val.asScalar() orelse default;
}

fn getBool(map: Map, key: []const u8, default: bool) bool {
    const val = map.get(key) orelse return default;
    if (val == .boolean) return val.boolean;
    const s = val.asScalar() orelse return default;
    if (std.mem.eql(u8, s, "true") or std.mem.eql(u8, s, "yes")) return true;
    if (std.mem.eql(u8, s, "false") or std.mem.eql(u8, s, "no")) return false;
    return default;
}

fn getInt(map: Map, key: []const u8, default: i64) i64 {
    const val = map.get(key) orelse return default;
    const s = val.asScalar() orelse return default;
    return std.fmt.parseInt(i64, s, 10) catch default;
}

fn getList(map: Map, key: []const u8) ?[]Value {
    const val = map.get(key) orelse return null;
    return val.asList();
}

fn getMap(map: Map, key: []const u8) ?Map {
    const val = map.get(key) orelse return null;
    return val.asMap();
}

// ── Config loading ───────────────────────────────────────────────────

pub fn load(alloc: Allocator, path: []const u8) !Config {
    const raw = std.fs.cwd().readFileAlloc(alloc, path, 1024 * 1024) catch |err| {
        std.log.err("failed to read config file '{s}': {}", .{ path, err });
        return error.ConfigReadFailed;
    };
    defer alloc.free(raw);

    const expanded = expandEnvVars(alloc, raw) catch |err| {
        std.log.err("failed to expand environment variables: {}", .{err});
        return err;
    };
    defer alloc.free(expanded);

    var y: yaml.Yaml = .{ .source = expanded };
    y.load(alloc) catch |err| {
        if (err == error.ParseFailure) {
            std.log.err("invalid YAML in config file", .{});
        }
        return error.ConfigParseFailed;
    };
    defer y.deinit(alloc);

    if (y.docs.items.len == 0) {
        std.log.err("empty config file", .{});
        return error.ConfigParseFailed;
    }

    const root_map = y.docs.items[0].asMap() orelse {
        std.log.err("config root must be a mapping", .{});
        return error.ConfigParseFailed;
    };

    return parseConfig(alloc, root_map);
}

fn parseConfig(alloc: Allocator, root: Map) !Config {
    const version = try getStr(root, "version");
    if (!std.mem.eql(u8, version, "1")) {
        std.log.err("unsupported config version: {s}", .{version});
        return error.UnsupportedVersion;
    }

    // Parse proxmox section
    const pve_map = getMap(root, "proxmox") orelse {
        std.log.err("missing 'proxmox' section", .{});
        return error.ConfigParseFailed;
    };
    const pve_clusters_list = getList(pve_map, "clusters") orelse {
        std.log.err("missing 'proxmox.clusters'", .{});
        return error.ConfigParseFailed;
    };
    var pve_clusters = try alloc.alloc(ProxmoxCluster, pve_clusters_list.len);
    for (pve_clusters_list, 0..) |item, i| {
        const m = item.asMap() orelse return error.ConfigParseFailed;
        pve_clusters[i] = .{
            .name = try getStr(m, "name"),
            .endpoint = try getStr(m, "endpoint"),
            .token_id = try getStr(m, "token_id"),
            .token_secret = try getStr(m, "token_secret"),
            .tls_verify = getBool(m, "tls_verify", true),
        };
    }

    // Parse talos section
    const talos_map = getMap(root, "talos") orelse {
        std.log.err("missing 'talos' section", .{});
        return error.ConfigParseFailed;
    };
    const talos = TalosConfig{
        .config_path = try getStr(talos_map, "config_path"),
        .context = try getStr(talos_map, "context"),
    };

    // Parse clusters section
    const clusters_list = getList(root, "clusters") orelse {
        std.log.err("missing 'clusters' section", .{});
        return error.ConfigParseFailed;
    };
    var clusters = try alloc.alloc(ClusterConfig, clusters_list.len);
    for (clusters_list, 0..) |item, i| {
        const m = item.asMap() orelse return error.ConfigParseFailed;
        const nodes_list = getList(m, "nodes") orelse return error.ConfigParseFailed;
        var nodes = try alloc.alloc(NodeConfig, nodes_list.len);
        for (nodes_list, 0..) |n, j| {
            const nm = n.asMap() orelse return error.ConfigParseFailed;
            nodes[j] = .{
                .name = try getStr(nm, "name"),
                .role = try getStr(nm, "role"),
                .proxmox_vmid = getInt(nm, "proxmox_vmid", 0),
                .proxmox_node = try getStr(nm, "proxmox_node"),
                .ip = try getStr(nm, "ip"),
            };
        }
        clusters[i] = .{
            .name = try getStr(m, "name"),
            .proxmox_cluster = try getStr(m, "proxmox_cluster"),
            .endpoint = try getStr(m, "endpoint"),
            .nodes = nodes,
        };
    }

    // Parse optional tui section
    var tui_settings = TuiSettings{};
    if (getMap(root, "tui")) |tui_map| {
        if (getMap(tui_map, "storage")) |storage| {
            const w = getInt(storage, "warn_threshold", 10);
            const c = getInt(storage, "crit_threshold", 5);
            tui_settings.warn_threshold = @intCast(@max(0, @min(100, w)));
            tui_settings.crit_threshold = @intCast(@max(0, @min(100, c)));
        }
        if (getMap(tui_map, "backups")) |backups| {
            const d = getInt(backups, "stale_days", 30);
            tui_settings.stale_days = @intCast(@max(0, d));
        }
        const ri = getStrOr(tui_map, "refresh_interval", "30s");
        tui_settings.refresh_interval_ms = parseDurationMs(ri);
    }

    return .{
        .version = version,
        .proxmox = .{ .clusters = pve_clusters },
        .talos = talos,
        .clusters = clusters,
        .tui_settings = tui_settings,
    };
}

// ── Utility functions ────────────────────────────────────────────────

/// Parse a duration string like "5m", "30s", "1h" into milliseconds.
pub fn parseDurationMs(s: []const u8) u64 {
    if (s.len == 0) return 30_000;
    const suffix = s[s.len - 1];
    const num_str = s[0 .. s.len - 1];
    const num = std.fmt.parseInt(u64, num_str, 10) catch return 30_000;
    return switch (suffix) {
        's' => num * 1_000,
        'm' => num * 60_000,
        'h' => num * 3_600_000,
        else => 30_000,
    };
}

/// Expand `${VAR}` references in a string using environment variables.
pub fn expandEnvVars(alloc: Allocator, input: []const u8) ![]const u8 {
    var result: std.ArrayListUnmanaged(u8) = .empty;
    errdefer result.deinit(alloc);

    var i: usize = 0;
    while (i < input.len) {
        if (i + 1 < input.len and input[i] == '$' and input[i + 1] == '{') {
            const start = i + 2;
            const end = std.mem.indexOfScalarPos(u8, input, start, '}') orelse {
                return error.UnterminatedEnvVar;
            };
            const var_name = input[start..end];
            const val = std.posix.getenv(var_name) orelse {
                std.log.err("environment variable not set: {s}", .{var_name});
                return error.EnvVarNotSet;
            };
            try result.appendSlice(alloc, val);
            i = end + 1;
        } else {
            try result.append(alloc, input[i]);
            i += 1;
        }
    }
    return result.toOwnedSlice(alloc);
}

/// Discover the config file path using standard search order.
pub fn discover() ![]const u8 {
    if (std.posix.getenv("PVT_CONFIG")) |p| {
        std.fs.cwd().access(p, .{}) catch {};
        return p;
    }
    std.fs.cwd().access("pvt.yaml", .{}) catch return error.ConfigNotFound;
    return "pvt.yaml";
}

// ── Tests ────────────────────────────────────────────────────────────

test "parseDurationMs" {
    const expect = std.testing.expect;
    try expect(parseDurationMs("30s") == 30_000);
    try expect(parseDurationMs("5m") == 300_000);
    try expect(parseDurationMs("1h") == 3_600_000);
    try expect(parseDurationMs("") == 30_000);
    try expect(parseDurationMs("bad") == 30_000);
}

test "expandEnvVars basic" {
    const alloc = std.testing.allocator;
    const result = try expandEnvVars(alloc, "hello world");
    defer alloc.free(result);
    try std.testing.expectEqualStrings("hello world", result);
}

test "TuiSettings defaults" {
    const s = TuiSettings{};
    try std.testing.expect(s.warn_threshold == 10);
    try std.testing.expect(s.crit_threshold == 5);
    try std.testing.expect(s.stale_days == 30);
    try std.testing.expect(s.refresh_interval_ms == 30_000);
}
