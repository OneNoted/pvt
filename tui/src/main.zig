const std = @import("std");
const config = @import("config.zig");
const App = @import("app.zig").App;

pub fn main() !void {
    var gpa_impl: std.heap.GeneralPurposeAllocator(.{}) = .init;
    defer _ = gpa_impl.deinit();
    const alloc = gpa_impl.allocator();

    const config_path = parseArgs() catch |err| {
        if (err == error.HelpRequested) return;
        return err;
    };

    const cfg = config.load(alloc, config_path) catch |err| {
        std.log.err("configuration error: {}", .{err});
        std.process.exit(1);
    };

    var app = App.init(alloc, cfg) catch |err| {
        std.log.err("failed to initialize TUI: {}", .{err});
        std.process.exit(1);
    };
    defer app.deinit(alloc);

    app.run(alloc) catch |err| {
        std.log.err("runtime error: {}", .{err});
        std.process.exit(1);
    };
}

fn parseArgs() ![]const u8 {
    var args = std.process.args();
    _ = args.skip(); // program name

    while (args.next()) |arg| {
        if (std.mem.eql(u8, arg, "--config") or std.mem.eql(u8, arg, "-c")) {
            return args.next() orelse {
                std.log.err("--config requires a path argument", .{});
                return error.MissingArgument;
            };
        }
        if (std.mem.eql(u8, arg, "--help") or std.mem.eql(u8, arg, "-h")) {
            _ = std.posix.write(std.posix.STDOUT_FILENO,
                \\vitui - TUI for pvt cluster management
                \\
                \\Usage: vitui [options]
                \\
                \\Options:
                \\  -c, --config <path>  Path to pvt.yaml config file
                \\  -h, --help           Show this help message
                \\
                \\If --config is not specified, vitui searches:
                \\  $PVT_CONFIG, ./pvt.yaml, ~/.config/pvt/config.yaml
                \\
            ) catch {};
            return error.HelpRequested;
        }
    }

    // No --config flag: try to discover
    return config.discover() catch {
        std.log.err("no config file found (use --config or set $PVT_CONFIG)", .{});
        return error.MissingArgument;
    };
}

test {
    _ = config;
}
