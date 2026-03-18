const std = @import("std");
const config = @import("../config.zig");
const http = @import("http_client.zig");
const Allocator = std.mem.Allocator;

pub const PodMetrics = struct {
    pod: []const u8,
    namespace: []const u8,
    cpu_cores: f64, // fractional cores
    memory_bytes: f64,
    net_rx_bytes_sec: f64,
    net_tx_bytes_sec: f64,
};

pub const NodeMetrics = struct {
    instance: []const u8,
    cpu_usage: f64, // 0.0 - 1.0
    mem_used: f64, // bytes
    mem_total: f64, // bytes
};

pub const MetricsClient = struct {
    allocator: Allocator,
    endpoint: []const u8, // e.g. "http://prometheus.monitoring.svc:9090"
    available: bool = false,

    pub fn init(allocator: Allocator, kubeconfig: []const u8) MetricsClient {
        // Autodetect metrics endpoint by querying known service names
        const candidates = [_]struct { ns: []const u8, svc: []const u8, port: []const u8 }{
            .{ .ns = "monitoring", .svc = "vmsingle-victoria-metrics-victoria-metrics-single-server", .port = "8428" },
            .{ .ns = "monitoring", .svc = "vmselect", .port = "8481" },
            .{ .ns = "monitoring", .svc = "prometheus-server", .port = "9090" },
            .{ .ns = "monitoring", .svc = "prometheus-operated", .port = "9090" },
            .{ .ns = "observability", .svc = "prometheus-server", .port = "9090" },
            .{ .ns = "observability", .svc = "prometheus-operated", .port = "9090" },
        };

        for (candidates) |c| {
            const endpoint = detectEndpoint(allocator, kubeconfig, c.ns, c.svc, c.port);
            if (endpoint) |ep| {
                return .{
                    .allocator = allocator,
                    .endpoint = ep,
                    .available = true,
                };
            }
        }

        return .{
            .allocator = allocator,
            .endpoint = "",
            .available = false,
        };
    }

    fn detectEndpoint(allocator: Allocator, kubeconfig: []const u8, ns: []const u8, svc: []const u8, port: []const u8) ?[]const u8 {
        // Use kubectl to check if the service exists
        var argv: std.ArrayListUnmanaged([]const u8) = .empty;
        defer argv.deinit(allocator);
        argv.appendSlice(allocator, &.{
            "kubectl", "get", "svc", svc, "-n", ns,
            "--kubeconfig", kubeconfig,
            "--no-headers", "-o", "name",
        }) catch return null;

        const result = std.process.Child.run(.{
            .allocator = allocator,
            .argv = argv.items,
            .max_output_bytes = 4096,
        }) catch return null;
        defer allocator.free(result.stderr);
        defer allocator.free(result.stdout);

        const term = result.term;
        if (term == .Exited and term.Exited == 0 and result.stdout.len > 0) {
            return std.fmt.allocPrint(allocator, "http://{s}.{s}.svc:{s}", .{ svc, ns, port }) catch null;
        }
        return null;
    }

    /// Query pod CPU usage via PromQL.
    pub fn getPodCpu(self: *MetricsClient) []PodMetricValue {
        if (!self.available) return &.{};
        return self.queryPromQL(
            "sum(rate(container_cpu_usage_seconds_total{container!=\"\",pod!=\"\"}[5m])) by (pod, namespace)",
        );
    }

    /// Query pod memory usage via PromQL.
    pub fn getPodMemory(self: *MetricsClient) []PodMetricValue {
        if (!self.available) return &.{};
        return self.queryPromQL(
            "sum(container_memory_working_set_bytes{container!=\"\",pod!=\"\"}) by (pod, namespace)",
        );
    }

    /// Query pod network rx via PromQL.
    pub fn getPodNetRx(self: *MetricsClient) []PodMetricValue {
        if (!self.available) return &.{};
        return self.queryPromQL(
            "sum(rate(container_network_receive_bytes_total{pod!=\"\"}[5m])) by (pod, namespace)",
        );
    }

    /// Query pod network tx via PromQL.
    pub fn getPodNetTx(self: *MetricsClient) []PodMetricValue {
        if (!self.available) return &.{};
        return self.queryPromQL(
            "sum(rate(container_network_transmit_bytes_total{pod!=\"\"}[5m])) by (pod, namespace)",
        );
    }

    /// Query node CPU usage via PromQL.
    pub fn getNodeCpu(self: *MetricsClient) []PodMetricValue {
        if (!self.available) return &.{};
        return self.queryPromQL(
            "1 - avg(rate(node_cpu_seconds_total{mode=\"idle\"}[5m])) by (instance)",
        );
    }

    /// Query node memory usage via PromQL.
    pub fn getNodeMemUsed(self: *MetricsClient) []PodMetricValue {
        if (!self.available) return &.{};
        return self.queryPromQL(
            "node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes",
        );
    }

    /// Query node total memory via PromQL.
    pub fn getNodeMemTotal(self: *MetricsClient) []PodMetricValue {
        if (!self.available) return &.{};
        return self.queryPromQL("node_memory_MemTotal_bytes");
    }

    pub const PodMetricValue = struct {
        labels: std.json.ObjectMap,
        value: f64,
    };

    fn queryPromQL(self: *MetricsClient, query: []const u8) []PodMetricValue {
        const alloc = self.allocator;

        // URL-encode the query
        var encoded: std.ArrayListUnmanaged(u8) = .empty;
        defer encoded.deinit(alloc);
        for (query) |c| {
            switch (c) {
                ' ' => encoded.appendSlice(alloc, "%20") catch return &.{},
                '"' => encoded.appendSlice(alloc, "%22") catch return &.{},
                '{' => encoded.appendSlice(alloc, "%7B") catch return &.{},
                '}' => encoded.appendSlice(alloc, "%7D") catch return &.{},
                '!' => encoded.appendSlice(alloc, "%21") catch return &.{},
                '[' => encoded.appendSlice(alloc, "%5B") catch return &.{},
                ']' => encoded.appendSlice(alloc, "%5D") catch return &.{},
                '=' => encoded.appendSlice(alloc, "%3D") catch return &.{},
                else => encoded.append(alloc, c) catch return &.{},
            }
        }

        const url = std.fmt.allocPrint(alloc, "{s}/api/v1/query?query={s}", .{
            self.endpoint,
            encoded.items,
        }) catch return &.{};
        defer alloc.free(url);

        // Use curl to query (unauthenticated, in-cluster)
        var argv: std.ArrayListUnmanaged([]const u8) = .empty;
        defer argv.deinit(alloc);
        argv.appendSlice(alloc, &.{
            "curl", "-s", "-f", "--max-time", "5", url,
        }) catch return &.{};

        const result = std.process.Child.run(.{
            .allocator = alloc,
            .argv = argv.items,
            .max_output_bytes = 1024 * 1024,
        }) catch return &.{};
        defer alloc.free(result.stderr);

        const term = result.term;
        if (!(term == .Exited and term.Exited == 0)) {
            alloc.free(result.stdout);
            return &.{};
        }
        defer alloc.free(result.stdout);

        return self.parsePromResponse(result.stdout);
    }

    fn parsePromResponse(self: *MetricsClient, body: []const u8) []PodMetricValue {
        const alloc = self.allocator;
        var parsed = std.json.parseFromSlice(std.json.Value, alloc, body, .{
            .ignore_unknown_fields = true,
            .allocate = .alloc_always,
        }) catch return &.{};
        defer parsed.deinit();

        const root = switch (parsed.value) {
            .object => |obj| obj,
            else => return &.{},
        };

        const data = switch (root.get("data") orelse return &.{}) {
            .object => |obj| obj,
            else => return &.{},
        };

        const results_arr = switch (data.get("result") orelse return &.{}) {
            .array => |arr| arr.items,
            else => return &.{},
        };

        var out: std.ArrayListUnmanaged(PodMetricValue) = .empty;
        for (results_arr) |item| {
            const obj = switch (item) {
                .object => |o| o,
                else => continue,
            };

            // metric labels
            const metric = switch (obj.get("metric") orelse continue) {
                .object => |o| o,
                else => continue,
            };

            // value is [timestamp, "value_string"]
            const value_arr = switch (obj.get("value") orelse continue) {
                .array => |arr| arr.items,
                else => continue,
            };
            if (value_arr.len < 2) continue;

            const val_str = switch (value_arr[1]) {
                .string => |s| s,
                else => continue,
            };
            const val = std.fmt.parseFloat(f64, val_str) catch continue;

            // Clone metric labels so they survive parsed.deinit()
            var cloned_labels = std.json.ObjectMap.init(alloc);
            var it = metric.iterator();
            while (it.next()) |entry| {
                const key = alloc.dupe(u8, entry.key_ptr.*) catch continue;
                const label_val = switch (entry.value_ptr.*) {
                    .string => |s| std.json.Value{ .string = alloc.dupe(u8, s) catch continue },
                    else => continue,
                };
                cloned_labels.put(key, label_val) catch continue;
            }

            out.append(alloc, .{
                .labels = cloned_labels,
                .value = val,
            }) catch continue;
        }

        return out.toOwnedSlice(alloc) catch &.{};
    }

    pub fn deinit(self: *MetricsClient) void {
        if (self.available and self.endpoint.len > 0) {
            self.allocator.free(self.endpoint);
        }
    }
};
