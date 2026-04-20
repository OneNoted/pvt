use std::time::{SystemTime, UNIX_EPOCH};

pub fn truncate(value: &str, max: usize) -> String {
    if value.chars().count() <= max {
        return value.to_string();
    }
    if max <= 1 {
        return "…".to_string();
    }
    let mut out = value.chars().take(max - 1).collect::<String>();
    out.push('…');
    out
}

pub fn format_bytes(bytes: i64) -> String {
    let bytes = bytes.max(0) as f64;
    const KIB: f64 = 1024.0;
    const MIB: f64 = 1024.0 * 1024.0;
    const GIB: f64 = 1024.0 * 1024.0 * 1024.0;
    const TIB: f64 = 1024.0 * 1024.0 * 1024.0 * 1024.0;

    if bytes >= TIB {
        format!("{:.1} TiB", bytes / TIB)
    } else if bytes >= GIB {
        format!("{:.1} GiB", bytes / GIB)
    } else if bytes >= MIB {
        format!("{:.1} MiB", bytes / MIB)
    } else {
        format!("{:.0} KiB", bytes / KIB)
    }
}

pub fn format_rate(bytes_per_sec: f64) -> String {
    const KIB: f64 = 1024.0;
    const MIB: f64 = 1024.0 * 1024.0;
    if bytes_per_sec >= MIB {
        format!("{:.1} MiB/s", bytes_per_sec / MIB)
    } else if bytes_per_sec >= KIB {
        format!("{:.1} KiB/s", bytes_per_sec / KIB)
    } else {
        format!("{:.0} B/s", bytes_per_sec)
    }
}

pub fn format_epoch(epoch: i64) -> String {
    if epoch <= 0 {
        return "unknown".to_string();
    }
    #[allow(deprecated)]
    {
        use std::time::Duration;
        let secs = Duration::from_secs(epoch as u64);
        let t = UNIX_EPOCH + secs;
        format_system_time(t)
    }
}

pub fn format_system_time(time: SystemTime) -> String {
    let Ok(duration) = time.duration_since(UNIX_EPOCH) else {
        return "unknown".to_string();
    };
    let secs = duration.as_secs() as i64;
    chrono_like(secs)
}

fn chrono_like(epoch: i64) -> String {
    const SECS_PER_DAY: i64 = 86_400;
    let days = epoch.div_euclid(SECS_PER_DAY);
    let seconds = epoch.rem_euclid(SECS_PER_DAY);
    let (year, month, day) = civil_from_days(days);
    let hour = seconds / 3600;
    let minute = (seconds % 3600) / 60;
    format!("{year:04}-{month:02}-{day:02} {hour:02}:{minute:02}")
}

fn civil_from_days(days: i64) -> (i32, u32, u32) {
    let z = days + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = z - era * 146_097;
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let y = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = mp + if mp < 10 { 3 } else { -9 };
    let year = y + if m <= 2 { 1 } else { 0 };
    (year as i32, m as u32, d as u32)
}

pub fn age_days(epoch: i64) -> u32 {
    let now = SystemTime::now();
    let then = UNIX_EPOCH + std::time::Duration::from_secs(epoch.max(0) as u64);
    let Ok(delta) = now.duration_since(then) else {
        return 0;
    };
    (delta.as_secs() / 86_400) as u32
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn duration_helpers_format_bytes() {
        assert_eq!(format_bytes(1024), "1 KiB");
        assert_eq!(format_bytes(1024 * 1024), "1.0 MiB");
    }

    #[test]
    fn truncate_adds_ellipsis() {
        assert_eq!(truncate("abcdef", 4), "abc…");
    }
}
