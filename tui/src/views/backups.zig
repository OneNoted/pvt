const std = @import("std");
const vaxis = @import("vaxis");
const poll = @import("../poll.zig");

pub const DeleteAction = struct {
    proxmox_cluster: []const u8,
    node: []const u8,
    storage: []const u8,
    volid: []const u8,
};

pub const BackupView = struct {
    selected: u16 = 0,
    scroll: u16 = 0,
    num_backups: u16 = 0,
    stale_days: u32,
    allocator: std.mem.Allocator,

    // Total row count across both sections (for navigation)
    total_rows: u16 = 0,

    // Confirmation dialog state
    show_confirm: bool = false,
    pending_idx: ?u16 = null,
    pending_delete: ?DeleteAction = null,

    // Set by handleKey when user confirms deletion
    delete_action: ?DeleteAction = null,

    // Search/filter state
    filter_active: bool = false,
    filter_buf: [64]u8 = undefined,
    filter_len: u8 = 0,

    const pve_col_header = "  VM Name         VMID    Date              Size         Storage       Age";
    const k8s_col_header = "  Name                    Namespace      Source    Status       Schedule         Last Run";

    pub fn init(allocator: std.mem.Allocator, stale_days: u32) BackupView {
        return .{
            .stale_days = stale_days,
            .allocator = allocator,
        };
    }

    pub fn handleKey(self: *BackupView, key: vaxis.Key) void {
        // Confirmation dialog intercepts all input
        if (self.show_confirm) {
            if (key.matches('y', .{})) {
                self.delete_action = self.pending_delete;
                self.pending_delete = null;
                self.pending_idx = null;
                self.show_confirm = false;
            } else if (key.matches('n', .{}) or key.matches(vaxis.Key.escape, .{})) {
                self.show_confirm = false;
                self.clearPendingDelete();
                self.pending_idx = null;
            }
            return;
        }

        // Filter input mode intercepts all input
        if (self.filter_active) {
            if (key.matches(vaxis.Key.escape, .{}) or key.matches(vaxis.Key.enter, .{})) {
                if (key.matches(vaxis.Key.escape, .{})) {
                    self.filter_len = 0; // Clear filter on Esc
                }
                self.filter_active = false;
            } else if (key.matches(vaxis.Key.backspace, .{})) {
                if (self.filter_len > 0) self.filter_len -= 1;
            } else if (key.text) |text| {
                for (text) |c| {
                    if (self.filter_len < self.filter_buf.len) {
                        self.filter_buf[self.filter_len] = c;
                        self.filter_len += 1;
                    }
                }
            }
            return;
        }

        if (key.matches('/', .{})) {
            self.filter_active = true;
            return;
        }

        // Esc clears active filter when not in input mode
        if (key.matches(vaxis.Key.escape, .{})) {
            if (self.filter_len > 0) {
                self.filter_len = 0;
                self.selected = 0;
            }
            return;
        }

        if (self.total_rows == 0) return;

        if (key.matches('j', .{}) or key.matches(vaxis.Key.down, .{})) {
            if (self.selected < self.total_rows - 1) self.selected += 1;
        } else if (key.matches('k', .{}) or key.matches(vaxis.Key.up, .{})) {
            if (self.selected > 0) self.selected -= 1;
        } else if (key.matches('g', .{})) {
            self.selected = 0;
        } else if (key.matches('G', .{ .shift = true })) {
            if (self.total_rows > 0) self.selected = self.total_rows - 1;
        } else if (key.matches('d', .{})) {
            // Only allow deletion on PVE backup rows
            if (self.selected < self.num_backups) {
                self.clearPendingDelete();
                self.pending_idx = self.selected;
                self.show_confirm = true;
                self.delete_action = null;
            }
        }
    }

    pub fn draw(
        self: *BackupView,
        win: vaxis.Window,
        backups: []const poll.BackupRow,
        k8s_backups: []const poll.K8sBackupRow,
    ) void {
        // Apply filter
        const filter = if (self.filter_len > 0) self.filter_buf[0..self.filter_len] else "";

        // Count filtered rows
        var pve_count: u16 = 0;
        for (backups) |b| {
            if (self.matchesFilter(b, filter)) pve_count += 1;
        }
        var k8s_count: u16 = 0;
        for (k8s_backups) |b| {
            if (self.matchesK8sFilter(b, filter)) k8s_count += 1;
        }

        self.num_backups = pve_count;
        self.total_rows = pve_count + k8s_count;

        if (self.total_rows == 0) {
            self.selected = 0;
            self.scroll = 0;
            if (filter.len > 0) {
                drawCentered(win, "No backups matching filter");
            } else {
                drawCentered(win, "No backups found");
            }
            self.drawFilterBar(win);
            return;
        }

        // Clamp selection
        if (self.selected >= self.total_rows) self.selected = self.total_rows - 1;

        const footer_rows = self.filterBarRows();
        const content_height = win.height -| footer_rows;
        if (content_height == 0) {
            self.scroll = 0;
            self.drawFilterBar(win);
            return;
        }

        const visible_rows = calcVisibleRows(content_height, pve_count, k8s_count);
        if (self.selected < self.scroll) {
            self.scroll = self.selected;
        } else if (self.selected >= self.scroll + visible_rows) {
            self.scroll = self.selected - visible_rows + 1;
        }

        if (self.scroll >= self.total_rows) self.scroll = self.total_rows - 1;
        const end_idx = self.scroll +| visible_rows;

        var current_row: u16 = 0;

        // PVE Backups section
        if (pve_count > 0) {
            var pve_header_buf: [48]u8 = undefined;
            const pve_header = std.fmt.bufPrint(&pve_header_buf, " PVE Backups ({d})", .{pve_count}) catch " PVE Backups";
            const hdr_style: vaxis.Style = .{ .fg = .{ .index = 6 }, .bg = .{ .index = 8 }, .bold = true };
            _ = win.print(&.{.{ .text = pve_header, .style = hdr_style }}, .{
                .row_offset = current_row,
                .wrap = .none,
            });
            current_row += 1;

            const col_hdr_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bold = true };
            _ = win.print(&.{.{ .text = pve_col_header, .style = col_hdr_style }}, .{
                .row_offset = current_row,
                .wrap = .none,
            });
            current_row += 1;

            var pve_idx: u16 = 0;
            for (backups) |b| {
                if (!self.matchesFilter(b, filter)) continue;
                const logical_idx = pve_idx;
                pve_idx += 1;
                if (logical_idx < self.scroll) continue;
                if (logical_idx >= end_idx) continue;
                if (current_row >= content_height) break;
                const is_selected = (logical_idx == self.selected);
                drawBackupRow(win, current_row, b, is_selected, self.stale_days);
                current_row += 1;
            }
        }

        // K8s Backups section
        if (k8s_count > 0) {
            if (pve_count > 0 and current_row < content_height -| 3) {
                // Separator
                current_row += 1;
            }

            var k8s_header_buf: [48]u8 = undefined;
            const k8s_header = std.fmt.bufPrint(&k8s_header_buf, " K8s Backups ({d})", .{k8s_count}) catch " K8s Backups";
            if (current_row < content_height -| 1) {
                const hdr_style: vaxis.Style = .{ .fg = .{ .index = 5 }, .bg = .{ .index = 8 }, .bold = true };
                _ = win.print(&.{.{ .text = k8s_header, .style = hdr_style }}, .{
                    .row_offset = current_row,
                    .wrap = .none,
                });
                current_row += 1;
            }

            if (current_row < content_height) {
                const col_hdr_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bold = true };
                _ = win.print(&.{.{ .text = k8s_col_header, .style = col_hdr_style }}, .{
                    .row_offset = current_row,
                    .wrap = .none,
                });
                current_row += 1;
            }

            var k8s_idx: u16 = 0;
            for (k8s_backups) |b| {
                if (!self.matchesK8sFilter(b, filter)) continue;
                const logical_idx = pve_count + k8s_idx;
                k8s_idx += 1;
                if (logical_idx < self.scroll) continue;
                if (logical_idx >= end_idx) continue;
                if (current_row >= content_height) break;
                const is_selected = (logical_idx == self.selected);
                drawK8sRow(win, current_row, b, is_selected);
                current_row += 1;
            }
        } else if (pve_count > 0 and current_row < content_height -| 1) {
            // Show "no K8s providers" hint
            current_row += 1;
            const hint_style: vaxis.Style = .{ .fg = .{ .index = 8 } };
            _ = win.print(&.{.{ .text = " K8s Backups: No providers detected", .style = hint_style }}, .{
                .row_offset = current_row,
                .wrap = .none,
            });
        }

        // Filter bar at bottom
        self.drawFilterBar(win);

        // Confirmation dialog overlay
        if (self.show_confirm) {
            if (self.pending_delete == null) {
                if (self.pending_idx) |idx| {
                    if (self.filteredBackupIndex(backups, idx)) |actual_idx| {
                        self.pending_delete = self.actionFromBackup(backups[actual_idx]) catch null;
                    } else {
                        self.show_confirm = false;
                        self.pending_idx = null;
                    }
                }
            }
            if (self.pending_delete) |action| {
                self.drawConfirmDialog(win, action.volid);
            }
        }
    }

    fn drawFilterBar(self: *BackupView, win: vaxis.Window) void {
        if (!self.filter_active and self.filter_len == 0) return;

        const bar_row = win.height -| 1;
        const filter_text = self.filter_buf[0..self.filter_len];

        if (self.filter_active) {
            var buf: [80]u8 = undefined;
            const line = std.fmt.bufPrint(&buf, " / filter: {s}_", .{filter_text}) catch " / filter: ";
            _ = win.print(&.{.{ .text = line, .style = .{
                .fg = .{ .index = 6 },
                .bg = .{ .index = 8 },
                .bold = true,
            } }}, .{
                .row_offset = bar_row,
                .wrap = .none,
            });
        } else if (self.filter_len > 0) {
            var buf: [80]u8 = undefined;
            const line = std.fmt.bufPrint(&buf, " filter: {s} (/ to edit, Esc to clear)", .{filter_text}) catch "";
            _ = win.print(&.{.{ .text = line, .style = .{
                .fg = .{ .index = 8 },
            } }}, .{
                .row_offset = bar_row,
                .wrap = .none,
            });
        }
    }

    fn matchesFilter(self: *BackupView, b: poll.BackupRow, filter: []const u8) bool {
        _ = self;
        if (filter.len == 0) return true;
        if (containsInsensitive(b.vm_name, filter)) return true;
        if (containsInsensitive(b.vmid, filter)) return true;
        if (containsInsensitive(b.storage, filter)) return true;
        if (containsInsensitive(b.date_str, filter)) return true;
        return false;
    }

    fn matchesK8sFilter(self: *BackupView, b: poll.K8sBackupRow, filter: []const u8) bool {
        _ = self;
        if (filter.len == 0) return true;
        if (containsInsensitive(b.name, filter)) return true;
        if (containsInsensitive(b.namespace, filter)) return true;
        if (containsInsensitive(b.source_type, filter)) return true;
        if (containsInsensitive(b.status, filter)) return true;
        return false;
    }

    fn drawBackupRow(win: vaxis.Window, row: u16, b: poll.BackupRow, selected: bool, stale_days: u32) void {
        const bg: vaxis.Color = if (selected) .{ .index = 4 } else .default;
        const base_fg: vaxis.Color = if (selected)
            .{ .index = 0 }
        else if (b.age_days > stale_days * 2)
            .{ .index = 1 } // red: very stale
        else if (b.is_stale)
            .{ .index = 3 } // yellow: stale
        else
            .{ .index = 7 }; // normal

        const style: vaxis.Style = .{ .fg = base_fg, .bg = bg };

        var age_buf: [16]u8 = undefined;
        const age_str = std.fmt.bufPrint(&age_buf, "{d}d", .{b.age_days}) catch "?d";

        var buf: [256]u8 = undefined;
        const line = std.fmt.bufPrint(&buf, "  {s:<16} {s:<7} {s:<17} {s:<12} {s:<13} {s}", .{
            truncate(b.vm_name, 16),
            truncate(b.vmid, 7),
            truncate(b.date_str, 17),
            truncate(b.size_str, 12),
            truncate(b.storage, 13),
            age_str,
        }) catch return;

        _ = win.print(&.{.{ .text = line, .style = style }}, .{
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn drawK8sRow(win: vaxis.Window, row: u16, b: poll.K8sBackupRow, selected: bool) void {
        const bg: vaxis.Color = if (selected) .{ .index = 4 } else .default;
        const fg: vaxis.Color = if (selected) .{ .index = 0 } else .{ .index = 7 };
        const style: vaxis.Style = .{ .fg = fg, .bg = bg };

        var buf: [256]u8 = undefined;
        const line = std.fmt.bufPrint(&buf, "  {s:<24} {s:<14} {s:<9} {s:<12} {s:<16} {s}", .{
            truncate(b.name, 24),
            truncate(b.namespace, 14),
            truncate(b.source_type, 9),
            truncate(b.status, 12),
            truncate(b.schedule, 16),
            truncate(b.last_run, 20),
        }) catch return;

        _ = win.print(&.{.{ .text = line, .style = style }}, .{
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn drawConfirmDialog(self: *BackupView, win: vaxis.Window, volid: []const u8) void {
        _ = self;
        const box_w: u16 = 52;
        const box_h: u16 = 7;
        const x: i17 = @intCast(if (win.width > box_w) (win.width - box_w) / 2 else 0);
        const y: i17 = @intCast(if (win.height > box_h) (win.height - box_h) / 2 else 0);

        const dialog = win.child(.{
            .x_off = x,
            .y_off = y,
            .width = box_w,
            .height = box_h,
            .border = .{ .where = .all, .style = .{ .fg = .{ .index = 1 } } },
        });

        dialog.fill(.{ .style = .{ .bg = .{ .index = 0 } } });

        const title_style: vaxis.Style = .{ .fg = .{ .index = 1 }, .bg = .{ .index = 0 }, .bold = true };
        const text_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bg = .{ .index = 0 } };
        const hint_style: vaxis.Style = .{ .fg = .{ .index = 8 }, .bg = .{ .index = 0 } };

        _ = dialog.print(&.{.{ .text = "  Delete Backup?", .style = title_style }}, .{
            .row_offset = 0,
            .wrap = .none,
        });

        var name_buf: [48]u8 = undefined;
        const name_line = std.fmt.bufPrint(&name_buf, "  {s}", .{truncate(volid, 46)}) catch "  ?";
        _ = dialog.print(&.{.{ .text = name_line, .style = text_style }}, .{
            .row_offset = 2,
            .wrap = .none,
        });

        _ = dialog.print(&.{.{ .text = "  y: confirm   n/Esc: cancel", .style = hint_style }}, .{
            .row_offset = 4,
            .wrap = .none,
        });
    }

    /// Check if there's a pending delete action and consume it.
    pub fn consumeDeleteAction(self: *BackupView) ?DeleteAction {
        if (self.delete_action != null) {
            self.pending_idx = null;
            const action = self.delete_action.?;
            self.delete_action = null;
            return action;
        }
        return null;
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

    fn calcHeaderRows(pve_count: u16, k8s_count: u16) u16 {
        var rows: u16 = 0;
        if (pve_count > 0) rows += 2;
        if (k8s_count > 0) {
            if (pve_count > 0) rows += 1;
            rows += 2;
        }
        return rows;
    }

    fn filterBarRows(self: *BackupView) u16 {
        return if (self.filter_active or self.filter_len > 0) 1 else 0;
    }

    fn calcVisibleRows(content_height: u16, pve_count: u16, k8s_count: u16) u16 {
        if (content_height == 0) return 0;
        const header_rows = calcHeaderRows(pve_count, k8s_count);
        return @max(@as(u16, 1), content_height -| header_rows);
    }

    fn filteredBackupIndex(self: *BackupView, backups: []const poll.BackupRow, filtered_idx: u16) ?u16 {
        const filter = if (self.filter_len > 0) self.filter_buf[0..self.filter_len] else "";
        var matched: u16 = 0;
        for (backups, 0..) |b, i| {
            if (!self.matchesFilter(b, filter)) continue;
            if (matched == filtered_idx) return @intCast(i);
            matched += 1;
        }
        return null;
    }

    fn actionFromBackup(self: *BackupView, backup: poll.BackupRow) !DeleteAction {
        const proxmox_cluster = try self.allocator.dupe(u8, backup.proxmox_cluster);
        errdefer self.allocator.free(proxmox_cluster);
        const node = try self.allocator.dupe(u8, backup.node);
        errdefer self.allocator.free(node);
        const storage = try self.allocator.dupe(u8, backup.storage);
        errdefer self.allocator.free(storage);
        const volid = try self.allocator.dupe(u8, backup.volid);
        return .{
            .proxmox_cluster = proxmox_cluster,
            .node = node,
            .storage = storage,
            .volid = volid,
        };
    }

    fn clearPendingDelete(self: *BackupView) void {
        if (self.pending_delete) |action| {
            self.allocator.free(action.proxmox_cluster);
            self.allocator.free(action.node);
            self.allocator.free(action.storage);
            self.allocator.free(action.volid);
            self.pending_delete = null;
        }
    }
};

/// Case-insensitive substring check (ASCII only).
fn containsInsensitive(haystack: []const u8, needle: []const u8) bool {
    if (needle.len == 0) return true;
    if (needle.len > haystack.len) return false;
    const limit = haystack.len - needle.len + 1;
    for (0..limit) |i| {
        var match = true;
        for (0..needle.len) |j| {
            if (toLower(haystack[i + j]) != toLower(needle[j])) {
                match = false;
                break;
            }
        }
        if (match) return true;
    }
    return false;
}

fn toLower(c: u8) u8 {
    return if (c >= 'A' and c <= 'Z') c + 32 else c;
}
