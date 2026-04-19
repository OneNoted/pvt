use crate::models::Snapshot;
use crate::util::truncate;
use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    Frame,
    layout::{Constraint, Rect},
    style::{Color, Modifier, Style},
    widgets::{Block, Borders, Cell, Paragraph, Row, Table, TableState},
};

#[derive(Default)]
pub struct ClusterViewState {
    pub table: TableState,
}

impl ClusterViewState {
    pub fn handle_key(&mut self, key: KeyEvent, row_count: usize) {
        if row_count == 0 {
            self.table.select(None);
            return;
        }
        let selected = self.table.selected().unwrap_or(0);
        let next = match key.code {
            KeyCode::Char('j') | KeyCode::Down => selected.saturating_add(1).min(row_count - 1),
            KeyCode::Char('k') | KeyCode::Up => selected.saturating_sub(1),
            KeyCode::Char('g') => 0,
            KeyCode::Char('G') => row_count - 1,
            _ => selected,
        };
        self.table.select(Some(next));
    }
}

pub fn render(frame: &mut Frame, area: Rect, state: &mut ClusterViewState, snapshot: &Snapshot) {
    if snapshot.cluster_rows.is_empty() {
        frame.render_widget(
            Paragraph::new("No cluster data available")
                .block(Block::default().borders(Borders::ALL).title("Cluster")),
            area,
        );
        return;
    }

    let rows = snapshot.cluster_rows.iter().map(|row| {
        Row::new(vec![
            Cell::from(truncate(&row.name, 16)),
            Cell::from(truncate(&row.role, 12)),
            Cell::from(truncate(&row.ip, 16)),
            Cell::from(truncate(&row.pve_node, 12)),
            Cell::from(row.vmid.clone()),
            Cell::from(truncate(&row.talos_version, 12)),
            Cell::from(truncate(&row.kubernetes_version, 12)),
            Cell::from(truncate(&row.etcd, 10)),
            Cell::from(truncate(&row.health, 12)),
        ])
    });

    let table = Table::new(
        rows,
        [
            Constraint::Length(16),
            Constraint::Length(12),
            Constraint::Length(16),
            Constraint::Length(12),
            Constraint::Length(8),
            Constraint::Length(12),
            Constraint::Length(12),
            Constraint::Length(10),
            Constraint::Min(10),
        ],
    )
    .header(
        Row::new(vec![
            "Name", "Role", "IP", "PVE Node", "VMID", "Talos", "K8s", "Etcd", "Health",
        ])
        .style(
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        ),
    )
    .block(Block::default().borders(Borders::ALL).title("Cluster"))
    .row_highlight_style(
        Style::default()
            .bg(Color::Blue)
            .fg(Color::Black)
            .add_modifier(Modifier::BOLD),
    );

    if state.table.selected().is_none() {
        state.table.select(Some(0));
    }
    frame.render_stateful_widget(table, area, &mut state.table);
}
