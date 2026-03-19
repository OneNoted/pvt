const std = @import("std");
const vaxis = @import("vaxis");
const poll = @import("../poll.zig");

const Table = vaxis.widgets.Table;

const Section = enum { pools, disks };

pub const StorageView = struct {
    active_section: Section = .pools,

    // Pool section state
    pool_selected: u16 = 0,
    pool_scroll: u16 = 0,
    num_pools: u16 = 0,

    // Disk section state
    disk_table_ctx: Table.TableContext,
    num_disks: u16 = 0,

    // Thresholds
    warn_threshold: u8,
    crit_threshold: u8,

    const pool_header = "  Pool Name       Node         Type        Used         Total        Usage              Status";
    const pool_header_sep = " ─────────────────────────────────────────────────────────────────────────────────────────────";

    pub fn init(warn: u8, crit: u8) StorageView {
        return .{
            .warn_threshold = warn,
            .crit_threshold = crit,
            .disk_table_ctx = .{
                .active = false,
                .selected_bg = .{ .index = 4 },
                .selected_fg = .{ .index = 0 },
                .active_bg = .{ .index = 4 },
                .active_fg = .{ .index = 0 },
                .hdr_bg_1 = .{ .index = 8 },
                .hdr_bg_2 = .{ .index = 8 },
                .row_bg_1 = .default,
                .row_bg_2 = .default,
                .col_width = .dynamic_fill,
                .header_names = .{ .custom = &.{ "VM Name", "VMID", "Node", "Size" } },
            },
        };
    }

    pub fn handleKey(self: *StorageView, key: vaxis.Key) void {
        if (key.matches(vaxis.Key.tab, .{})) {
            self.toggleSection();
            return;
        }

        switch (self.active_section) {
            .pools => self.handlePoolKey(key),
            .disks => self.handleDiskKey(key),
        }
    }

    fn toggleSection(self: *StorageView) void {
        self.active_section = if (self.active_section == .pools) .disks else .pools;
        self.disk_table_ctx.active = (self.active_section == .disks);
    }

    fn handlePoolKey(self: *StorageView, key: vaxis.Key) void {
        if (self.num_pools == 0) return;
        if (key.matches('j', .{}) or key.matches(vaxis.Key.down, .{})) {
            if (self.pool_selected < self.num_pools - 1) self.pool_selected += 1;
        } else if (key.matches('k', .{}) or key.matches(vaxis.Key.up, .{})) {
            if (self.pool_selected > 0) self.pool_selected -= 1;
        } else if (key.matches('g', .{})) {
            self.pool_selected = 0;
        } else if (key.matches('G', .{ .shift = true })) {
            if (self.num_pools > 0) self.pool_selected = self.num_pools - 1;
        }
    }

    fn handleDiskKey(self: *StorageView, key: vaxis.Key) void {
        if (self.num_disks == 0) return;
        if (key.matches('j', .{}) or key.matches(vaxis.Key.down, .{})) {
            if (self.disk_table_ctx.row < self.num_disks - 1) self.disk_table_ctx.row += 1;
        } else if (key.matches('k', .{}) or key.matches(vaxis.Key.up, .{})) {
            if (self.disk_table_ctx.row > 0) self.disk_table_ctx.row -= 1;
        } else if (key.matches('g', .{})) {
            self.disk_table_ctx.row = 0;
        } else if (key.matches('G', .{ .shift = true })) {
            if (self.num_disks > 0) self.disk_table_ctx.row = self.num_disks - 1;
        }
    }

    pub fn draw(
        self: *StorageView,
        alloc: std.mem.Allocator,
        win: vaxis.Window,
        pools: []const poll.StoragePoolRow,
        disks: []const poll.VmDiskRow,
    ) void {
        self.num_pools = @intCast(pools.len);
        self.num_disks = @intCast(disks.len);

        if (pools.len == 0 and disks.len == 0) {
            drawEmpty(win);
            return;
        }

        // Clamp selections
        if (self.pool_selected >= self.num_pools and self.num_pools > 0)
            self.pool_selected = self.num_pools - 1;
        if (self.disk_table_ctx.row >= self.num_disks and self.num_disks > 0)
            self.disk_table_ctx.row = self.num_disks - 1;

        // Split layout: pools get top portion, disks get bottom
        const sep_row: u16 = @intCast(@max(4, @min(win.height -| 6, (win.height * 55) / 100)));
        const pools_win = win.child(.{ .height = sep_row });
        const disks_win = win.child(.{ .y_off = @intCast(sep_row + 1), .height = win.height -| sep_row -| 1 });

        // Separator line
        self.drawSeparator(win, sep_row);

        // Draw sections
        self.drawPools(pools_win, pools);
        self.drawDisks(alloc, disks_win, disks);
    }

    fn drawSeparator(self: *StorageView, win: vaxis.Window, row: u16) void {
        const label = if (self.active_section == .disks) " VM Disks (active) " else " VM Disks ";
        const style: vaxis.Style = .{ .fg = .{ .index = 8 } };
        const active_style: vaxis.Style = .{ .fg = .{ .index = 6 }, .bold = true };
        _ = win.print(&.{.{
            .text = label,
            .style = if (self.active_section == .disks) active_style else style,
        }}, .{ .row_offset = row, .wrap = .none });
    }

    fn drawPools(self: *StorageView, win: vaxis.Window, pools: []const poll.StoragePoolRow) void {
        if (pools.len == 0) return;

        const is_active = (self.active_section == .pools);
        const hdr_style: vaxis.Style = .{ .fg = .{ .index = 6 }, .bg = .{ .index = 8 }, .bold = true };
        const hdr_label = if (is_active) " Storage Pools (active)" else " Storage Pools";

        // Header
        _ = win.print(&.{.{ .text = hdr_label, .style = hdr_style }}, .{ .wrap = .none });

        // Column headers (row 1)
        const col_hdr_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bold = true };
        _ = win.print(&.{.{ .text = pool_header, .style = col_hdr_style }}, .{
            .row_offset = 1,
            .wrap = .none,
        });

        // Scrolling
        const visible_rows = win.height -| 2;
        if (self.pool_selected < self.pool_scroll) {
            self.pool_scroll = self.pool_selected;
        } else if (self.pool_selected >= self.pool_scroll + visible_rows) {
            self.pool_scroll = self.pool_selected - visible_rows + 1;
        }

        // Rows
        var row_idx: u16 = 0;
        const start = self.pool_scroll;
        const end: u16 = @intCast(@min(pools.len, start + visible_rows));
        for (pools[start..end]) |p| {
            const display_row = row_idx + 2; // after header + col headers
            const is_selected = is_active and (start + row_idx == self.pool_selected);

            self.drawPoolRow(win, display_row, p, is_selected);
            row_idx += 1;
        }
    }

    fn drawPoolRow(self: *StorageView, win: vaxis.Window, row: u16, p: poll.StoragePoolRow, selected: bool) void {
        const bg: vaxis.Color = if (selected) .{ .index = 4 } else .default;
        const fg: vaxis.Color = if (selected) .{ .index = 0 } else .{ .index = 7 };
        const style: vaxis.Style = .{ .fg = fg, .bg = bg };

        // Format: "  name             node         type        used         total        [bar] pct%   status"
        var buf: [256]u8 = undefined;
        const line = std.fmt.bufPrint(&buf, "  {s:<16} {s:<12} {s:<10} {s:<12} {s:<12}", .{
            truncate(p.name, 16),
            truncate(p.node, 12),
            truncate(p.pool_type, 10),
            truncate(p.used_str, 12),
            truncate(p.total_str, 12),
        }) catch return;

        _ = win.print(&.{.{ .text = line, .style = style }}, .{
            .row_offset = row,
            .wrap = .none,
        });

        // Usage bar at column 66
        self.drawUsageBar(win, row, 66, p.usage_pct, bg);

        // Status after bar (col ~84)
        const status_style: vaxis.Style = .{
            .fg = if (std.mem.eql(u8, p.status, "available")) .{ .index = 2 } else .{ .index = 3 },
            .bg = bg,
        };
        _ = win.print(&.{.{ .text = truncate(p.status, 10), .style = status_style }}, .{
            .col_offset = 85,
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn drawUsageBar(self: *StorageView, win: vaxis.Window, row: u16, col: u16, pct: f64, bg: vaxis.Color) void {
        const bar_width: u16 = 10;
        const remaining = 100.0 - pct;
        const bar_color: vaxis.Color = if (remaining < @as(f64, @floatFromInt(self.crit_threshold)))
            .{ .index = 1 } // red
        else if (remaining < @as(f64, @floatFromInt(self.warn_threshold)))
            .{ .index = 3 } // yellow
        else
            .{ .index = 2 }; // green

        const filled: u16 = @intFromFloat(@min(
            @as(f64, @floatFromInt(bar_width)),
            @round(pct / 100.0 * @as(f64, @floatFromInt(bar_width))),
        ));
        const empty_count = bar_width - filled;

        // Build fill/empty strings from Unicode blocks
        var fill_buf: [30]u8 = undefined;
        var fill_len: usize = 0;
        for (0..filled) |_| {
            const ch = "\u{2588}";
            @memcpy(fill_buf[fill_len..][0..ch.len], ch);
            fill_len += ch.len;
        }

        var empty_buf: [30]u8 = undefined;
        var empty_len: usize = 0;
        for (0..empty_count) |_| {
            const ch = "\u{2591}";
            @memcpy(empty_buf[empty_len..][0..ch.len], ch);
            empty_len += ch.len;
        }

        var pct_buf: [8]u8 = undefined;
        const pct_str = std.fmt.bufPrint(&pct_buf, "] {d:>3.0}%", .{pct}) catch "] ?%";

        _ = win.print(&.{
            .{ .text = "[", .style = .{ .fg = .{ .index = 7 }, .bg = bg } },
            .{ .text = fill_buf[0..fill_len], .style = .{ .fg = bar_color, .bg = bg } },
            .{ .text = empty_buf[0..empty_len], .style = .{ .fg = .{ .index = 8 }, .bg = bg } },
            .{ .text = pct_str, .style = .{ .fg = .{ .index = 7 }, .bg = bg } },
        }, .{
            .col_offset = col,
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn drawDisks(self: *StorageView, alloc: std.mem.Allocator, win: vaxis.Window, disks: []const poll.VmDiskRow) void {
        if (disks.len == 0) {
            const msg = "No VM disk data";
            const c: u16 = if (win.width > msg.len) (win.width - @as(u16, @intCast(msg.len))) / 2 else 0;
            _ = win.print(&.{.{ .text = msg, .style = .{ .fg = .{ .index = 8 } } }}, .{
                .col_offset = c,
                .row_offset = win.height / 2,
                .wrap = .none,
            });
            return;
        }

        if (self.disk_table_ctx.row >= self.num_disks)
            self.disk_table_ctx.row = self.num_disks - 1;

        Table.drawTable(alloc, win, disks, &self.disk_table_ctx) catch {};
    }

    fn drawEmpty(win: vaxis.Window) void {
        const msg = "No storage data available";
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
