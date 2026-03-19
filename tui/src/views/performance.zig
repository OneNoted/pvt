const std = @import("std");
const vaxis = @import("vaxis");
const poll = @import("../poll.zig");

const SortColumn = enum { pod, namespace, cpu, memory, net_rx, net_tx };

pub const PerformanceView = struct {
    // Pod table navigation
    selected: u16 = 0,
    scroll: u16 = 0,
    num_pods: u16 = 0,

    // Sorting
    sort_col: SortColumn = .cpu,
    sort_asc: bool = false, // descending by default (highest first)

    // Namespace filter
    ns_filter: ?[]const u8 = null, // null = all namespaces
    ns_index: u16 = 0, // index into discovered namespaces (0 = all)

    const host_header = " Host Overview";
    const host_col_header = "  Node              CPU                             Memory";
    const pod_col_header = "  Pod                              Namespace        CPU       Memory       Net RX       Net TX";

    pub fn init() PerformanceView {
        return .{};
    }

    pub fn handleKey(self: *PerformanceView, key: vaxis.Key) void {
        if (key.matches('s', .{})) {
            self.cycleSortCol();
        } else if (key.matches('S', .{ .shift = true })) {
            self.sort_asc = !self.sort_asc;
        } else if (key.matches('n', .{})) {
            self.ns_index +%= 1; // wraps, clamped in draw
        } else if (key.matches('j', .{}) or key.matches(vaxis.Key.down, .{})) {
            if (self.num_pods > 0 and self.selected < self.num_pods - 1) self.selected += 1;
        } else if (key.matches('k', .{}) or key.matches(vaxis.Key.up, .{})) {
            if (self.selected > 0) self.selected -= 1;
        } else if (key.matches('g', .{})) {
            self.selected = 0;
        } else if (key.matches('G', .{ .shift = true })) {
            if (self.num_pods > 0) self.selected = self.num_pods - 1;
        }
    }

    fn cycleSortCol(self: *PerformanceView) void {
        self.sort_col = switch (self.sort_col) {
            .pod => .namespace,
            .namespace => .cpu,
            .cpu => .memory,
            .memory => .net_rx,
            .net_rx => .net_tx,
            .net_tx => .pod,
        };
    }

    pub fn draw(
        self: *PerformanceView,
        alloc: std.mem.Allocator,
        win: vaxis.Window,
        hosts: []const poll.HostRow,
        pods: []const poll.PodMetricRow,
        metrics_available: bool,
    ) void {
        if (!metrics_available and hosts.len == 0) {
            drawCentered(win, "No metrics backend detected");
            return;
        }

        var current_row: u16 = 0;

        // Host overview section
        if (hosts.len > 0) {
            const hdr_style: vaxis.Style = .{ .fg = .{ .index = 6 }, .bg = .{ .index = 8 }, .bold = true };
            _ = win.print(&.{.{ .text = host_header, .style = hdr_style }}, .{
                .row_offset = current_row,
                .wrap = .none,
            });
            current_row += 1;

            const col_hdr_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bold = true };
            _ = win.print(&.{.{ .text = host_col_header, .style = col_hdr_style }}, .{
                .row_offset = current_row,
                .wrap = .none,
            });
            current_row += 1;

            for (hosts) |h| {
                if (current_row >= win.height -| 4) break;
                self.drawHostRow(win, current_row, h);
                current_row += 1;
            }
            current_row += 1; // spacing
        }

        // Pod metrics section
        if (!metrics_available) {
            if (current_row < win.height -| 2) {
                const hint: vaxis.Style = .{ .fg = .{ .index = 8 } };
                _ = win.print(&.{.{ .text = " Pod Metrics: No Prometheus/VictoriaMetrics detected", .style = hint }}, .{
                    .row_offset = current_row,
                    .wrap = .none,
                });
            }
            return;
        }

        // Discover namespaces and apply filter
        var namespaces: std.ArrayListUnmanaged([]const u8) = .empty;
        defer namespaces.deinit(alloc);
        for (pods) |p| {
            var found = false;
            for (namespaces.items) |ns| {
                if (std.mem.eql(u8, ns, p.namespace)) {
                    found = true;
                    break;
                }
            }
            if (!found) {
                namespaces.append(alloc, p.namespace) catch continue;
            }
        }

        // Sort namespaces alphabetically
        std.mem.sort([]const u8, namespaces.items, {}, struct {
            fn cmp(_: void, a: []const u8, b: []const u8) bool {
                return std.mem.order(u8, a, b) == .lt;
            }
        }.cmp);

        // Clamp namespace index (0 = all, 1..N = specific)
        const total_ns = namespaces.items.len;
        if (total_ns > 0 and self.ns_index > total_ns) {
            self.ns_index = 0;
        }

        const active_ns: ?[]const u8 = if (self.ns_index > 0 and self.ns_index <= total_ns)
            namespaces.items[self.ns_index - 1]
        else
            null;

        // Filter pods by namespace
        var filtered: std.ArrayListUnmanaged(poll.PodMetricRow) = .empty;
        defer filtered.deinit(alloc);
        for (pods) |p| {
            if (active_ns) |ns| {
                if (!std.mem.eql(u8, p.namespace, ns)) continue;
            }
            filtered.append(alloc, p) catch continue;
        }

        // Sort filtered pods
        self.sortPods(filtered.items);

        self.num_pods = @intCast(filtered.items.len);
        if (self.num_pods == 0) {
            self.selected = 0;
            self.scroll = 0;
        } else {
            if (self.selected >= self.num_pods) self.selected = self.num_pods - 1;
            if (self.scroll >= self.num_pods) self.scroll = self.num_pods - 1;
        }

        // Pod header
        {
            var hdr_buf: [64]u8 = undefined;
            const ns_label = if (active_ns) |ns| ns else "all";
            const pod_header = std.fmt.bufPrint(&hdr_buf, " Pod Metrics ({d}) [ns: {s}]", .{
                filtered.items.len, ns_label,
            }) catch " Pod Metrics";
            const hdr_style: vaxis.Style = .{ .fg = .{ .index = 5 }, .bg = .{ .index = 8 }, .bold = true };
            if (current_row < win.height -| 2) {
                _ = win.print(&.{.{ .text = pod_header, .style = hdr_style }}, .{
                    .row_offset = current_row,
                    .wrap = .none,
                });
                current_row += 1;
            }
        }

        // Sort indicator in column headers
        {
            const col_hdr_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bold = true };
            if (current_row < win.height -| 1) {
                _ = win.print(&.{.{ .text = pod_col_header, .style = col_hdr_style }}, .{
                    .row_offset = current_row,
                    .wrap = .none,
                });
                current_row += 1;
            }
        }

        // Scrolling
        const visible = win.height -| current_row -| 1;
        if (visible == 0) {
            self.scroll = 0;
            return;
        }
        if (self.selected < self.scroll) {
            self.scroll = self.selected;
        } else if (self.selected >= self.scroll + visible) {
            self.scroll = self.selected - visible + 1;
        }

        // Pod rows
        const start = self.scroll;
        const end: u16 = @intCast(@min(filtered.items.len, start + visible));
        var idx: u16 = 0;
        for (filtered.items[start..end]) |p| {
            if (current_row >= win.height -| 1) break;
            const is_selected = (start + idx == self.selected);
            drawPodRow(win, current_row, p, is_selected);
            current_row += 1;
            idx += 1;
        }

        // Status hints at bottom
        if (win.height > 1) {
            const sort_name = switch (self.sort_col) {
                .pod => "pod",
                .namespace => "ns",
                .cpu => "cpu",
                .memory => "mem",
                .net_rx => "rx",
                .net_tx => "tx",
            };
            const dir = if (self.sort_asc) "asc" else "desc";
            var hint_buf: [64]u8 = undefined;
            const hint = std.fmt.bufPrint(&hint_buf, " sort: {s} ({s})  s:cycle S:reverse n:namespace", .{ sort_name, dir }) catch "";
            _ = win.print(&.{.{ .text = hint, .style = .{ .fg = .{ .index = 8 } } }}, .{
                .row_offset = win.height - 1,
                .wrap = .none,
            });
        }
    }

    fn drawHostRow(self: *PerformanceView, win: vaxis.Window, row: u16, h: poll.HostRow) void {
        _ = self;
        const style: vaxis.Style = .{ .fg = .{ .index = 7 } };

        var buf: [128]u8 = undefined;
        const text = std.fmt.bufPrint(&buf, "  {s:<18}", .{truncate(h.name, 18)}) catch return;
        _ = win.print(&.{.{ .text = text, .style = style }}, .{
            .row_offset = row,
            .wrap = .none,
        });

        // CPU bar at col 20
        drawBar(win, row, 20, h.cpu_pct, 15);

        // Memory bar at col 52
        var mem_buf: [32]u8 = undefined;
        const mem_label = std.fmt.bufPrint(&mem_buf, " {s}/{s}", .{
            truncate(h.mem_used_str, 12),
            truncate(h.mem_total_str, 12),
        }) catch "";
        drawBar(win, row, 52, h.mem_pct, 15);

        _ = win.print(&.{.{ .text = mem_label, .style = .{ .fg = .{ .index = 8 } } }}, .{
            .col_offset = 74,
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn drawBar(win: vaxis.Window, row: u16, col: u16, pct: f64, width: u16) void {
        const bar_color: vaxis.Color = if (pct > 90)
            .{ .index = 1 } // red
        else if (pct > 70)
            .{ .index = 3 } // yellow
        else
            .{ .index = 2 }; // green

        const filled: u16 = @intFromFloat(@min(
            @as(f64, @floatFromInt(width)),
            @round(pct / 100.0 * @as(f64, @floatFromInt(width))),
        ));
        const empty_count = width - filled;

        var fill_buf: [60]u8 = undefined;
        var fill_len: usize = 0;
        for (0..filled) |_| {
            const ch = "\u{2588}";
            @memcpy(fill_buf[fill_len..][0..ch.len], ch);
            fill_len += ch.len;
        }

        var empty_buf: [60]u8 = undefined;
        var empty_len: usize = 0;
        for (0..empty_count) |_| {
            const ch = "\u{2591}";
            @memcpy(empty_buf[empty_len..][0..ch.len], ch);
            empty_len += ch.len;
        }

        var pct_buf: [8]u8 = undefined;
        const pct_str = std.fmt.bufPrint(&pct_buf, "] {d:>3.0}%", .{pct}) catch "] ?%";

        _ = win.print(&.{
            .{ .text = "[", .style = .{ .fg = .{ .index = 7 } } },
            .{ .text = fill_buf[0..fill_len], .style = .{ .fg = bar_color } },
            .{ .text = empty_buf[0..empty_len], .style = .{ .fg = .{ .index = 8 } } },
            .{ .text = pct_str, .style = .{ .fg = .{ .index = 7 } } },
        }, .{
            .col_offset = col,
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn drawPodRow(win: vaxis.Window, row: u16, p: poll.PodMetricRow, selected: bool) void {
        const bg: vaxis.Color = if (selected) .{ .index = 4 } else .default;
        const fg: vaxis.Color = if (selected) .{ .index = 0 } else .{ .index = 7 };
        const style: vaxis.Style = .{ .fg = fg, .bg = bg };

        var buf: [256]u8 = undefined;
        const line = std.fmt.bufPrint(&buf, "  {s:<33} {s:<16} {s:<9} {s:<12} {s:<12} {s}", .{
            truncate(p.pod, 33),
            truncate(p.namespace, 16),
            truncate(p.cpu_str, 9),
            truncate(p.mem_str, 12),
            truncate(p.net_rx_str, 12),
            truncate(p.net_tx_str, 12),
        }) catch return;

        _ = win.print(&.{.{ .text = line, .style = style }}, .{
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn sortPods(self: *PerformanceView, items: []poll.PodMetricRow) void {
        const asc = self.sort_asc;
        switch (self.sort_col) {
            .pod => std.mem.sort(poll.PodMetricRow, items, asc, struct {
                fn cmp(ascending: bool, a: poll.PodMetricRow, b: poll.PodMetricRow) bool {
                    const ord = std.mem.order(u8, a.pod, b.pod);
                    return if (ascending) ord == .lt else ord == .gt;
                }
            }.cmp),
            .namespace => std.mem.sort(poll.PodMetricRow, items, asc, struct {
                fn cmp(ascending: bool, a: poll.PodMetricRow, b: poll.PodMetricRow) bool {
                    const ord = std.mem.order(u8, a.namespace, b.namespace);
                    return if (ascending) ord == .lt else ord == .gt;
                }
            }.cmp),
            .cpu => std.mem.sort(poll.PodMetricRow, items, asc, struct {
                fn cmp(ascending: bool, a: poll.PodMetricRow, b: poll.PodMetricRow) bool {
                    return if (ascending) a.cpu_cores < b.cpu_cores else a.cpu_cores > b.cpu_cores;
                }
            }.cmp),
            .memory => std.mem.sort(poll.PodMetricRow, items, asc, struct {
                fn cmp(ascending: bool, a: poll.PodMetricRow, b: poll.PodMetricRow) bool {
                    return if (ascending) a.mem_bytes < b.mem_bytes else a.mem_bytes > b.mem_bytes;
                }
            }.cmp),
            .net_rx => {
                std.mem.sort(poll.PodMetricRow, items, asc, struct {
                    fn cmp(ascending: bool, a: poll.PodMetricRow, b: poll.PodMetricRow) bool {
                        return if (ascending)
                            a.net_rx_bytes_sec < b.net_rx_bytes_sec
                        else
                            a.net_rx_bytes_sec > b.net_rx_bytes_sec;
                    }
                }.cmp);
            },
            .net_tx => {
                std.mem.sort(poll.PodMetricRow, items, asc, struct {
                    fn cmp(ascending: bool, a: poll.PodMetricRow, b: poll.PodMetricRow) bool {
                        return if (ascending)
                            a.net_tx_bytes_sec < b.net_tx_bytes_sec
                        else
                            a.net_tx_bytes_sec > b.net_tx_bytes_sec;
                    }
                }.cmp);
            },
        }
    }

    fn drawCentered(win: vaxis.Window, msg: []const u8) void {
        const col: u16 = if (win.width > msg.len) (win.width - @as(u16, @intCast(msg.len))) / 2 else 0;
        _ = win.print(&.{.{ .text = msg, .style = .{ .fg = .{ .index = 8 } } }}, .{
            .col_offset = col,
            .row_offset = win.height / 2,
            .wrap = .none,
        });
    }

    fn truncate(s: []const u8, max: usize) []const u8 {
        return if (s.len > max) s[0..max] else s;
    }
};
