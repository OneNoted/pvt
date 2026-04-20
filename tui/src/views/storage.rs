use crate::models::Snapshot;
use crate::util::truncate;
use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    Frame,
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    widgets::{Block, Borders, Cell, Paragraph, Row, Table, TableState},
};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum StorageSection {
    Pools,
    Disks,
}

pub struct StorageViewState {
    pub active_section: StorageSection,
    pub pool_table: TableState,
    pub disk_table: TableState,
}

impl Default for StorageViewState {
    fn default() -> Self {
        Self {
            active_section: StorageSection::Pools,
            pool_table: TableState::default(),
            disk_table: TableState::default(),
        }
    }
}

impl StorageViewState {
    pub fn handle_key(&mut self, key: KeyEvent, pool_count: usize, disk_count: usize) {
        match key.code {
            KeyCode::Left | KeyCode::Char('h') => self.active_section = StorageSection::Pools,
            KeyCode::Right | KeyCode::Char('l') => self.active_section = StorageSection::Disks,
            _ => match self.active_section {
                StorageSection::Pools => handle_table_nav(&mut self.pool_table, key, pool_count),
                StorageSection::Disks => handle_table_nav(&mut self.disk_table, key, disk_count),
            },
        }
    }
}

fn handle_table_nav(state: &mut TableState, key: KeyEvent, count: usize) {
    if count == 0 {
        state.select(None);
        return;
    }
    let selected = state.selected().unwrap_or(0);
    let next = match key.code {
        KeyCode::Char('j') | KeyCode::Down => selected.saturating_add(1).min(count - 1),
        KeyCode::Char('k') | KeyCode::Up => selected.saturating_sub(1),
        KeyCode::Char('g') => 0,
        KeyCode::Char('G') => count - 1,
        _ => selected,
    };
    state.select(Some(next));
}

pub fn render(
    frame: &mut Frame,
    area: Rect,
    state: &mut StorageViewState,
    snapshot: &Snapshot,
    warn: u8,
    crit: u8,
) {
    if snapshot.storage_pools.is_empty() && snapshot.vm_disks.is_empty() {
        frame.render_widget(
            Paragraph::new("No storage data available")
                .block(Block::default().borders(Borders::ALL).title("Storage")),
            area,
        );
        return;
    }

    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Percentage(55), Constraint::Percentage(45)])
        .split(area);

    let pool_rows = snapshot.storage_pools.iter().map(|pool| {
        let remaining = 100.0 - pool.usage_pct;
        let color = if remaining < crit as f64 {
            Color::Red
        } else if remaining < warn as f64 {
            Color::Yellow
        } else {
            Color::Green
        };
        Row::new(vec![
            Cell::from(truncate(&pool.name, 16)),
            Cell::from(truncate(&pool.node, 12)),
            Cell::from(truncate(&pool.pool_type, 10)),
            Cell::from(pool.used_str.clone()),
            Cell::from(pool.total_str.clone()),
            Cell::from(format!("{:>5.1}%", pool.usage_pct)).style(Style::default().fg(color)),
            Cell::from(truncate(&pool.status, 12)),
        ])
    });
    let pool_title = if state.active_section == StorageSection::Pools {
        "Storage Pools (active: h/l switch panes)"
    } else {
        "Storage Pools"
    };
    let pool_table = Table::new(
        pool_rows,
        [
            Constraint::Length(16),
            Constraint::Length(12),
            Constraint::Length(10),
            Constraint::Length(12),
            Constraint::Length(12),
            Constraint::Length(8),
            Constraint::Min(10),
        ],
    )
    .header(
        Row::new(vec![
            "Pool", "Node", "Type", "Used", "Total", "Usage", "Status",
        ])
        .style(
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        ),
    )
    .block(Block::default().borders(Borders::ALL).title(pool_title))
    .row_highlight_style(
        Style::default()
            .bg(Color::Blue)
            .fg(Color::Black)
            .add_modifier(Modifier::BOLD),
    );
    if state.pool_table.selected().is_none() && !snapshot.storage_pools.is_empty() {
        state.pool_table.select(Some(0));
    }
    frame.render_stateful_widget(pool_table, chunks[0], &mut state.pool_table);

    let disk_rows = snapshot.vm_disks.iter().map(|disk| {
        Row::new(vec![
            Cell::from(truncate(&disk.vm_name, 22)),
            Cell::from(disk.vmid.clone()),
            Cell::from(truncate(&disk.node, 12)),
            Cell::from(disk.size_str.clone()),
        ])
    });
    let disk_title = if state.active_section == StorageSection::Disks {
        "VM Disks (active: h/l switch panes)"
    } else {
        "VM Disks"
    };
    let disk_table = Table::new(
        disk_rows,
        [
            Constraint::Length(22),
            Constraint::Length(8),
            Constraint::Length(12),
            Constraint::Min(10),
        ],
    )
    .header(
        Row::new(vec!["VM Name", "VMID", "Node", "Size"]).style(
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        ),
    )
    .block(Block::default().borders(Borders::ALL).title(disk_title))
    .row_highlight_style(
        Style::default()
            .bg(Color::Blue)
            .fg(Color::Black)
            .add_modifier(Modifier::BOLD),
    );
    if state.disk_table.selected().is_none() && !snapshot.vm_disks.is_empty() {
        state.disk_table.select(Some(0));
    }
    frame.render_stateful_widget(disk_table, chunks[1], &mut state.disk_table);
}
