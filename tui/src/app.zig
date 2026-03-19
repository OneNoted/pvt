const std = @import("std");
const vaxis = @import("vaxis");
const config = @import("config.zig");
const poll = @import("poll.zig");
const ClusterView = @import("views/cluster.zig").ClusterView;
const StorageView = @import("views/storage.zig").StorageView;
const backups_view = @import("views/backups.zig");
const BackupView = backups_view.BackupView;
const DeleteAction = backups_view.DeleteAction;
const PerformanceView = @import("views/performance.zig").PerformanceView;
const proxmox_api = @import("api/proxmox.zig");

pub const Event = union(enum) {
    key_press: vaxis.Key,
    key_release: vaxis.Key,
    mouse: vaxis.Mouse,
    mouse_leave,
    focus_in,
    focus_out,
    paste_start,
    paste_end,
    paste: []const u8,
    color_report: vaxis.Color.Report,
    color_scheme: vaxis.Color.Scheme,
    winsize: vaxis.Winsize,
    cap_kitty_keyboard,
    cap_kitty_graphics,
    cap_rgb,
    cap_sgr_pixels,
    cap_unicode,
    cap_da1,
    cap_color_scheme_updates,
    cap_multi_cursor,
    // Custom events
    data_refresh,
};

pub const View = enum(u8) {
    cluster = 0,
    storage = 1,
    backups = 2,
    performance = 3,

    pub fn label(self: View) []const u8 {
        return switch (self) {
            .cluster => " 1:Cluster ",
            .storage => " 2:Storage ",
            .backups => " 3:Backups ",
            .performance => " 4:Perf ",
        };
    }
};

const min_width: u16 = 80;
const min_height: u16 = 24;

pub const App = struct {
    vx: vaxis.Vaxis,
    tty: vaxis.Tty,
    loop: vaxis.Loop(Event),
    cfg: config.Config,
    active_view: View = .cluster,
    show_help: bool = false,
    should_quit: bool = false,
    alloc: std.mem.Allocator,

    // Cluster view + polling
    cluster_view: ClusterView,
    cluster_state: *poll.ClusterState,
    // Storage view
    storage_view: StorageView,
    storage_state: *poll.StorageState,
    // Backup view
    backup_view: BackupView,
    backup_state: *poll.BackupState,
    // Performance view
    perf_view: PerformanceView,
    perf_state: *poll.PerfState,
    // Poller (shared)
    poller: *poll.Poller,

    tty_buf: [4096]u8 = undefined,

    pub fn init(alloc: std.mem.Allocator, cfg: config.Config) !App {
        const state = try alloc.create(poll.ClusterState);
        state.* = poll.ClusterState.init(alloc);

        const storage_state = try alloc.create(poll.StorageState);
        storage_state.* = poll.StorageState.init(alloc);

        const backup_state = try alloc.create(poll.BackupState);
        backup_state.* = poll.BackupState.init(alloc);

        const perf_state = try alloc.create(poll.PerfState);
        perf_state.* = poll.PerfState.init(alloc);

        const poller = try alloc.create(poll.Poller);
        // cfg pointer set in run() after App is at its final address
        poller.* = poll.Poller.init(alloc, state, storage_state, backup_state, perf_state, undefined, cfg.tui_settings.refresh_interval_ms);

        var app: App = .{
            .vx = try vaxis.init(alloc, .{}),
            .tty = undefined,
            .loop = undefined,
            .cfg = cfg,
            .alloc = alloc,
            .cluster_view = ClusterView.init(),
            .cluster_state = state,
            .storage_view = StorageView.init(cfg.tui_settings.warn_threshold, cfg.tui_settings.crit_threshold),
            .storage_state = storage_state,
            .backup_view = BackupView.init(cfg.tui_settings.stale_days),
            .backup_state = backup_state,
            .perf_view = PerformanceView.init(),
            .perf_state = perf_state,
            .poller = poller,
        };
        app.tty = try vaxis.Tty.init(&app.tty_buf);
        // `App` is returned by value, so pointer-bearing runtime fields must be
        // wired after the caller has the app at its final address.
        return app;
    }

    pub fn restoreTerminal(self: *App, alloc: std.mem.Allocator) void {
        // Signal poller to stop (non-blocking) so it can begin winding down
        self.poller.should_stop.store(true, .release);

        // `vaxis.Loop.stop()` wakes the reader by writing a device-status
        // query, which can hang shutdown if the terminal never answers it.
        // Mark the loop as quitting, restore the screen, then close the TTY.
        // The normal quit path exits the process immediately after this, so we
        // intentionally do not wait for background threads here.
        self.loop.should_quit = true;
        self.vx.deinit(alloc, self.tty.writer());
        self.tty.deinit();
    }

    pub fn deinit(self: *App, alloc: std.mem.Allocator) void {
        self.restoreTerminal(alloc);

        // Now wait for the poller thread to actually finish
        if (self.poller.thread) |t| {
            t.join();
            self.poller.thread = null;
        }

        self.cluster_state.deinit();
        self.storage_state.deinit();
        self.backup_state.deinit();
        self.perf_state.deinit();
        alloc.destroy(self.cluster_state);
        alloc.destroy(self.storage_state);
        alloc.destroy(self.backup_state);
        alloc.destroy(self.perf_state);
        alloc.destroy(self.poller);
    }

    pub fn run(self: *App, alloc: std.mem.Allocator) !void {
        // Now that self is at its final address, wire up runtime pointers.
        self.poller.cfg = &self.cfg;
        self.loop = .{ .tty = &self.tty, .vaxis = &self.vx };
        self.poller.setRefreshNotifier(self, postRefreshEvent);

        try self.loop.init();
        try self.loop.start();

        // Start background polling
        try self.poller.start();

        try self.vx.enterAltScreen(self.tty.writer());
        try self.vx.queryTerminal(self.tty.writer(), 1_000_000_000);

        while (!self.should_quit) {
            const event = self.loop.nextEvent();
            try self.handleEvent(alloc, event);
            if (self.should_quit) break;
            try self.draw();
            try self.vx.render(self.tty.writer());
        }
    }

    fn postRefreshEvent(context: *anyopaque) void {
        const self: *App = @ptrCast(@alignCast(context));
        _ = self.loop.tryPostEvent(.data_refresh);
    }

    fn handleEvent(self: *App, alloc: std.mem.Allocator, event: Event) !void {
        switch (event) {
            .key_press => |key| self.handleKey(key),
            .winsize => |ws| try self.vx.resize(alloc, self.tty.writer(), ws),
            .data_refresh => {}, // Just triggers redraw
            else => {},
        }
    }

    fn handleKey(self: *App, key: vaxis.Key) void {
        // Help overlay dismissal
        if (self.show_help) {
            if (key.matches('?', .{}) or key.matches(vaxis.Key.escape, .{})) {
                self.show_help = false;
            }
            return;
        }

        // Global keys
        if (key.matches('q', .{}) or key.matches('q', .{ .ctrl = true })) {
            self.should_quit = true;
            return;
        }
        if (key.matches('?', .{})) {
            self.show_help = true;
            return;
        }
        if (key.matches('r', .{})) {
            self.poller.triggerRefresh();
            return;
        }

        // View switching: 1-4
        if (key.matches('1', .{})) {
            self.active_view = .cluster;
        } else if (key.matches('2', .{})) {
            self.active_view = .storage;
        } else if (key.matches('3', .{})) {
            self.active_view = .backups;
        } else if (key.matches('4', .{})) {
            self.active_view = .performance;
        } else if (key.matches(vaxis.Key.tab, .{})) {
            self.cycleView();
        } else if (key.matches(vaxis.Key.tab, .{ .shift = true })) {
            self.cycleViewBack();
        } else {
            // Delegate to active view
            switch (self.active_view) {
                .cluster => self.cluster_view.handleKey(key),
                .storage => self.storage_view.handleKey(key),
                .backups => self.backup_view.handleKey(key),
                .performance => self.perf_view.handleKey(key),
            }
        }
    }

    fn cycleView(self: *App) void {
        const cur = @intFromEnum(self.active_view);
        self.active_view = @enumFromInt((cur + 1) % 4);
    }

    fn cycleViewBack(self: *App) void {
        const cur = @intFromEnum(self.active_view);
        self.active_view = @enumFromInt((cur + 3) % 4);
    }

    fn draw(self: *App) !void {
        const win = self.vx.window();
        win.clear();

        if (win.width < min_width or win.height < min_height) {
            self.drawMinSizeMessage(win);
            return;
        }

        // Top bar (row 0)
        const top_bar = win.child(.{ .height = 1 });
        self.drawTopBar(top_bar);

        // Status bar (last row)
        const status_bar = win.child(.{
            .y_off = @intCast(win.height -| 1),
            .height = 1,
        });
        self.drawStatusBar(status_bar);

        // Content area
        const content = win.child(.{
            .y_off = 1,
            .height = win.height -| 2,
        });
        self.drawContent(content);

        // Help overlay on top
        if (self.show_help) {
            self.drawHelpOverlay(win);
        }
    }

    fn drawMinSizeMessage(self: *App, win: vaxis.Window) void {
        _ = self;
        const msg = "Terminal too small (min 80x24)";
        const col: u16 = if (win.width > msg.len) (win.width - @as(u16, @intCast(msg.len))) / 2 else 0;
        const row: u16 = win.height / 2;
        _ = win.print(&.{.{ .text = msg, .style = .{ .fg = .{ .index = 1 } } }}, .{
            .col_offset = col,
            .row_offset = row,
        });
    }

    fn drawTopBar(self: *App, win: vaxis.Window) void {
        win.fill(.{ .style = .{ .bg = .{ .index = 8 } } });

        var col: u16 = 0;
        const views = [_]View{ .cluster, .storage, .backups, .performance };
        for (views) |view| {
            const lbl = view.label();
            const is_active = (view == self.active_view);
            const style: vaxis.Style = if (is_active)
                .{ .fg = .{ .index = 0 }, .bg = .{ .index = 4 }, .bold = true }
            else
                .{ .fg = .{ .index = 7 }, .bg = .{ .index = 8 } };

            _ = win.print(&.{.{ .text = lbl, .style = style }}, .{
                .col_offset = col,
                .wrap = .none,
            });
            col += @intCast(lbl.len);
        }

        const title = " vitui ";
        if (win.width > title.len + col) {
            const title_col: u16 = win.width - @as(u16, @intCast(title.len));
            _ = win.print(&.{.{ .text = title, .style = .{
                .fg = .{ .index = 6 },
                .bg = .{ .index = 8 },
                .bold = true,
            } }}, .{
                .col_offset = title_col,
                .wrap = .none,
            });
        }
    }

    fn drawStatusBar(self: *App, win: vaxis.Window) void {
        win.fill(.{ .style = .{ .bg = .{ .index = 8 } } });

        const bar_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bg = .{ .index = 8 } };

        // Left: keybinding hints
        const hint = " q:quit  ?:help  1-4:views  r:refresh  j/k:nav ";
        _ = win.print(&.{.{ .text = hint, .style = bar_style }}, .{ .wrap = .none });

        // Right: refresh status
        var buf: [64]u8 = undefined;
        const status_text = blk: {
            if (self.cluster_state.isLoading()) {
                break :blk "Loading...";
            }
            const last = self.cluster_state.getLastRefresh();
            if (last == 0) break :blk "";
            const now = std.time.timestamp();
            const ago = now - last;
            if (ago < 0) break :blk "";
            break :blk std.fmt.bufPrint(&buf, " {d}s ago ", .{ago}) catch "";
        };
        if (status_text.len > 0 and win.width > status_text.len + hint.len) {
            const status_col: u16 = win.width - @as(u16, @intCast(status_text.len));
            _ = win.print(&.{.{ .text = status_text, .style = bar_style }}, .{
                .col_offset = status_col,
                .wrap = .none,
            });
        }
    }

    fn drawContent(self: *App, win: vaxis.Window) void {
        switch (self.active_view) {
            .cluster => {
                self.cluster_state.lock();
                defer self.cluster_state.unlock();

                if (self.cluster_state.is_loading and self.cluster_state.rows.len == 0) {
                    self.drawPlaceholder(win, "Loading cluster data...");
                } else {
                    self.cluster_view.draw(self.alloc, win, self.cluster_state.rows);
                }
            },
            .storage => {
                self.storage_state.lock();
                defer self.storage_state.unlock();

                if (self.storage_state.is_loading and self.storage_state.pools.len == 0) {
                    self.drawPlaceholder(win, "Loading storage data...");
                } else {
                    self.storage_view.draw(self.alloc, win, self.storage_state.pools, self.storage_state.vm_disks);
                }
            },
            .backups => {
                var action: ?DeleteAction = null;
                self.backup_state.lock();
                {
                    defer self.backup_state.unlock();

                    if (self.backup_state.is_loading and
                        self.backup_state.backups.len == 0 and
                        self.backup_state.k8s_backups.len == 0)
                    {
                        self.drawPlaceholder(win, "Loading backup data...");
                    } else {
                        self.backup_view.draw(win, self.backup_state.backups, self.backup_state.k8s_backups);

                        // Copy action data while the backing rows are still locked.
                        if (self.backup_view.consumeDeleteAction(self.backup_state.backups)) |pending| {
                            const node = self.alloc.dupe(u8, pending.node) catch return;
                            const storage = self.alloc.dupe(u8, pending.storage) catch {
                                self.alloc.free(node);
                                return;
                            };
                            const volid = self.alloc.dupe(u8, pending.volid) catch {
                                self.alloc.free(storage);
                                self.alloc.free(node);
                                return;
                            };
                            action = .{
                                .node = node,
                                .storage = storage,
                                .volid = volid,
                            };
                        }
                    }
                }

                if (action) |owned_action| {
                    defer self.alloc.free(owned_action.node);
                    defer self.alloc.free(owned_action.storage);
                    defer self.alloc.free(owned_action.volid);
                    self.executeDelete(owned_action);
                }
            },
            .performance => {
                self.perf_state.lock();
                defer self.perf_state.unlock();

                if (self.perf_state.is_loading and self.perf_state.hosts.len == 0) {
                    self.drawPlaceholder(win, "Loading performance data...");
                } else {
                    self.perf_view.draw(
                        self.alloc,
                        win,
                        self.perf_state.hosts,
                        self.perf_state.pods,
                        self.perf_state.metrics_available,
                    );
                }
            },
        }
    }

    fn executeDelete(self: *App, action: DeleteAction) void {
        // Find matching PVE cluster config for this node
        for (self.cfg.proxmox.clusters) |pc| {
            var client = proxmox_api.ProxmoxClient.init(self.alloc, pc);
            defer client.deinit();
            client.deleteBackup(action.node, action.storage, action.volid) catch continue;
            // Trigger refresh to show updated list
            self.poller.triggerRefresh();
            return;
        }
    }

    fn drawPlaceholder(self: *App, win: vaxis.Window, label: []const u8) void {
        _ = self;
        const col: u16 = if (win.width > label.len) (win.width - @as(u16, @intCast(label.len))) / 2 else 0;
        const row: u16 = win.height / 2;
        _ = win.print(&.{.{ .text = label, .style = .{
            .fg = .{ .index = 6 },
            .bold = true,
        } }}, .{
            .col_offset = col,
            .row_offset = row,
            .wrap = .none,
        });
    }

    fn drawHelpOverlay(self: *App, win: vaxis.Window) void {
        const box_w: u16 = 48;
        const box_h: u16 = 22;
        const x: i17 = @intCast(if (win.width > box_w) (win.width - box_w) / 2 else 0);
        const y: i17 = @intCast(if (win.height > box_h) (win.height - box_h) / 2 else 0);

        const help_win = win.child(.{
            .x_off = x,
            .y_off = y,
            .width = box_w,
            .height = box_h,
            .border = .{ .where = .all, .style = .{ .fg = .{ .index = 4 } } },
        });

        help_win.fill(.{ .style = .{ .bg = .{ .index = 0 } } });

        const title_style: vaxis.Style = .{ .fg = .{ .index = 6 }, .bg = .{ .index = 0 }, .bold = true };
        const text_style: vaxis.Style = .{ .fg = .{ .index = 7 }, .bg = .{ .index = 0 } };
        const section_style: vaxis.Style = .{ .fg = .{ .index = 5 }, .bg = .{ .index = 0 }, .bold = true };

        var row: u16 = 0;
        const w = help_win;

        _ = w.print(&.{.{ .text = "             Keybindings", .style = title_style }}, .{ .row_offset = row, .wrap = .none });
        row += 1;

        const global = [_][]const u8{
            "  q           Quit",
            "  ?           Toggle help",
            "  1-4         Switch view",
            "  Tab/S-Tab   Next/Prev view",
            "  j/k         Navigate down/up",
            "  g/G         Top/Bottom",
            "  r           Refresh all data",
            "  Esc         Close overlay",
        };
        for (global) |line| {
            row += 1;
            if (row >= w.height) break;
            _ = w.print(&.{.{ .text = line, .style = text_style }}, .{ .row_offset = row, .wrap = .none });
        }

        // View-specific hints
        row += 1;
        if (row < w.height) {
            const view_title = switch (self.active_view) {
                .cluster => "  Cluster View",
                .storage => "  Storage View",
                .backups => "  Backups View",
                .performance => "  Performance View",
            };
            _ = w.print(&.{.{ .text = view_title, .style = section_style }}, .{ .row_offset = row, .wrap = .none });
            row += 1;
        }

        const view_lines: []const []const u8 = switch (self.active_view) {
            .cluster => &.{
                "  (no extra keys)",
            },
            .storage => &.{
                "  Tab         Switch pools/disks",
            },
            .backups => &.{
                "  d           Delete selected backup",
                "  /           Search/filter",
                "  Esc         Clear filter",
            },
            .performance => &.{
                "  s           Cycle sort column",
                "  S           Reverse sort direction",
                "  n           Cycle namespace filter",
            },
        };
        for (view_lines) |line| {
            if (row >= w.height) break;
            _ = w.print(&.{.{ .text = line, .style = text_style }}, .{ .row_offset = row, .wrap = .none });
            row += 1;
        }
    }
};
