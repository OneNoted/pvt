const std = @import("std");
const vaxis = @import("vaxis");
const poll = @import("../poll.zig");

const Table = vaxis.widgets.Table;
const Cell = vaxis.Cell;

pub const ClusterView = struct {
    table_ctx: Table.TableContext,
    num_rows: u16 = 0,

    pub fn init() ClusterView {
        return .{
            .table_ctx = .{
                .active = true,
                .selected_bg = .{ .index = 4 },
                .selected_fg = .{ .index = 0 },
                .active_bg = .{ .index = 4 },
                .active_fg = .{ .index = 0 },
                .hdr_bg_1 = .{ .index = 8 },
                .hdr_bg_2 = .{ .index = 8 },
                .row_bg_1 = .default,
                .row_bg_2 = .default,
                .col_width = .dynamic_fill,
                .header_names = .{ .custom = &.{
                    "Name", "Role", "IP", "PVE Node", "VMID", "Talos Ver", "K8s Ver", "Etcd", "Health",
                } },
            },
        };
    }

    pub fn handleKey(self: *ClusterView, key: vaxis.Key) void {
        if (self.num_rows == 0) return;

        if (key.matches('j', .{}) or key.matches(vaxis.Key.down, .{})) {
            if (self.table_ctx.row < self.num_rows - 1) {
                self.table_ctx.row += 1;
            }
        } else if (key.matches('k', .{}) or key.matches(vaxis.Key.up, .{})) {
            if (self.table_ctx.row > 0) {
                self.table_ctx.row -= 1;
            }
        } else if (key.matches('g', .{})) {
            // gg: go to top (single g for now)
            self.table_ctx.row = 0;
        } else if (key.matches('G', .{ .shift = true })) {
            // G: go to bottom
            if (self.num_rows > 0) {
                self.table_ctx.row = self.num_rows - 1;
            }
        }
    }

    pub fn draw(self: *ClusterView, alloc: std.mem.Allocator, win: vaxis.Window, rows: []const poll.NodeRow) void {
        self.num_rows = @intCast(rows.len);
        if (rows.len == 0) {
            self.drawEmpty(win);
            return;
        }

        // Clamp selected row
        if (self.table_ctx.row >= self.num_rows) {
            self.table_ctx.row = self.num_rows - 1;
        }

        Table.drawTable(alloc, win, rows, &self.table_ctx) catch {
            self.drawEmpty(win);
        };
    }

    fn drawEmpty(self: *ClusterView, win: vaxis.Window) void {
        _ = self;
        const msg = "No cluster data available";
        const col: u16 = if (win.width > msg.len) (win.width - @as(u16, @intCast(msg.len))) / 2 else 0;
        const row: u16 = win.height / 2;
        _ = win.print(&.{.{ .text = msg, .style = .{ .fg = .{ .index = 8 } } }}, .{
            .col_offset = col,
            .row_offset = row,
            .wrap = .none,
        });
    }
};
