use crate::models::{BackupRow, DeleteAction, Snapshot};
use crate::util::truncate;
use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    Frame,
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Cell, Clear, Paragraph, Row, Table, TableState, Wrap},
};

#[derive(Default)]
pub struct BackupsViewState {
    pub selected: usize,
    pub filter: String,
    pub filter_mode: bool,
    pub confirm_delete: bool,
    pub pending_delete: Option<DeleteAction>,
    pub pve_table: TableState,
}

impl BackupsViewState {
    pub fn handle_key(&mut self, key: KeyEvent, snapshot: &Snapshot) -> Option<DeleteAction> {
        if self.confirm_delete {
            match key.code {
                KeyCode::Char('y') => {
                    self.confirm_delete = false;
                    return self.pending_delete.take();
                }
                KeyCode::Char('n') | KeyCode::Esc => {
                    self.confirm_delete = false;
                    self.pending_delete = None;
                }
                _ => {}
            }
            return None;
        }

        if self.filter_mode {
            match key.code {
                KeyCode::Esc => {
                    self.filter.clear();
                    self.filter_mode = false;
                }
                KeyCode::Enter => self.filter_mode = false,
                KeyCode::Backspace => {
                    self.filter.pop();
                }
                KeyCode::Char(ch) => self.filter.push(ch),
                _ => {}
            }
            return None;
        }

        let filtered = filtered_pve(snapshot, &self.filter);
        let filtered_k8s = filtered_k8s(snapshot, &self.filter);
        let total = filtered.len() + filtered_k8s.len();
        if total == 0 {
            self.selected = 0;
        }

        match key.code {
            KeyCode::Char('/') => self.filter_mode = true,
            KeyCode::Esc => self.filter.clear(),
            KeyCode::Char('j') | KeyCode::Down => {
                if total > 0 {
                    self.selected = (self.selected + 1).min(total - 1);
                }
            }
            KeyCode::Char('k') | KeyCode::Up => {
                self.selected = self.selected.saturating_sub(1);
            }
            KeyCode::Char('g') => self.selected = 0,
            KeyCode::Char('G') => {
                if total > 0 {
                    self.selected = total - 1;
                }
            }
            KeyCode::Char('d') => {
                if self.selected < filtered.len() {
                    let row = filtered[self.selected];
                    self.pending_delete = Some(DeleteAction {
                        proxmox_cluster: row.proxmox_cluster.clone(),
                        node: row.node.clone(),
                        storage: row.storage.clone(),
                        volid: row.volid.clone(),
                    });
                    self.confirm_delete = true;
                }
            }
            _ => {}
        }
        None
    }
}

pub fn render(frame: &mut Frame, area: Rect, state: &mut BackupsViewState, snapshot: &Snapshot) {
    let pve = filtered_pve(snapshot, &state.filter);
    let k8s = filtered_k8s(snapshot, &state.filter);

    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Percentage(52),
            Constraint::Percentage(48),
            Constraint::Length(2),
        ])
        .split(area);

    let pve_rows = pve.iter().map(|row| {
        let age_style = if row.is_stale {
            Style::default().fg(Color::Yellow)
        } else {
            Style::default()
        };
        Row::new(vec![
            Cell::from(truncate(&row.vm_name, 18)),
            Cell::from(row.vmid.clone()),
            Cell::from(truncate(&row.date_str, 16)),
            Cell::from(row.size_str.clone()),
            Cell::from(truncate(&row.storage, 12)),
            Cell::from(format!("{}d", row.age_days)).style(age_style),
        ])
    });
    let pve_table = Table::new(
        pve_rows,
        [
            Constraint::Length(18),
            Constraint::Length(8),
            Constraint::Length(16),
            Constraint::Length(12),
            Constraint::Length(12),
            Constraint::Min(8),
        ],
    )
    .header(
        Row::new(vec!["VM Name", "VMID", "Date", "Size", "Storage", "Age"]).style(
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        ),
    )
    .block(Block::default().borders(Borders::ALL).title("PVE Backups"))
    .row_highlight_style(
        Style::default()
            .bg(Color::Blue)
            .fg(Color::Black)
            .add_modifier(Modifier::BOLD),
    );
    state
        .pve_table
        .select(if pve.is_empty() || state.selected >= pve.len() {
            None
        } else {
            Some(state.selected)
        });
    frame.render_stateful_widget(pve_table, chunks[0], &mut state.pve_table);

    let k8s_rows = k8s.iter().enumerate().map(|(index, row)| {
        let global_index = pve.len() + index;
        let mut style = Style::default();
        if global_index == state.selected {
            style = style
                .bg(Color::Blue)
                .fg(Color::Black)
                .add_modifier(Modifier::BOLD);
        }
        Row::new(vec![
            Cell::from(truncate(&row.name, 20)),
            Cell::from(truncate(&row.namespace, 14)),
            Cell::from(truncate(&row.source_type, 10)),
            Cell::from(truncate(&row.status, 12)),
            Cell::from(truncate(&row.schedule, 14)),
            Cell::from(truncate(&row.last_run, 16)),
        ])
        .style(style)
    });
    let k8s_table = Table::new(
        k8s_rows,
        [
            Constraint::Length(20),
            Constraint::Length(14),
            Constraint::Length(10),
            Constraint::Length(12),
            Constraint::Length(14),
            Constraint::Min(12),
        ],
    )
    .header(
        Row::new(vec![
            "Name",
            "Namespace",
            "Source",
            "Status",
            "Schedule",
            "Last Run",
        ])
        .style(
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        ),
    )
    .block(Block::default().borders(Borders::ALL).title("K8s Backups"));
    frame.render_widget(k8s_table, chunks[1]);

    let footer = if state.filter_mode {
        vec![
            Span::styled(
                "Filter: ",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(&state.filter),
            Span::styled(
                "  (Enter to apply, Esc to clear)",
                Style::default().fg(Color::DarkGray),
            ),
        ]
    } else {
        vec![
            Span::raw(format!(
                "filter={}  ",
                if state.filter.is_empty() {
                    "<none>"
                } else {
                    &state.filter
                }
            )),
            Span::styled(
                "/",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(" search  "),
            Span::styled(
                "d",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(" delete backup  "),
            Span::styled(
                "y/n",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(" confirm/cancel"),
        ]
    };
    frame.render_widget(
        Paragraph::new(Line::from(footer))
            .block(Block::default().borders(Borders::ALL).title("Backups")),
        chunks[2],
    );

    if state.confirm_delete {
        let popup = centered_rect(area, 60, 20);
        frame.render_widget(Clear, popup);
        let action = state.pending_delete.as_ref();
        let text = format!(
            "Delete backup?\n\n{}\n\nPress y to confirm or n to cancel.",
            action.map(|item| item.volid.as_str()).unwrap_or("unknown")
        );
        frame.render_widget(
            Paragraph::new(text).wrap(Wrap { trim: true }).block(
                Block::default()
                    .borders(Borders::ALL)
                    .title("Confirm delete"),
            ),
            popup,
        );
    }
}

fn filtered_pve<'a>(snapshot: &'a Snapshot, filter: &str) -> Vec<&'a BackupRow> {
    let needle = filter.to_lowercase();
    snapshot
        .backups
        .iter()
        .filter(|row| {
            needle.is_empty()
                || [
                    row.vm_name.as_str(),
                    row.vmid.as_str(),
                    row.storage.as_str(),
                    row.node.as_str(),
                    row.volid.as_str(),
                ]
                .iter()
                .any(|value| value.to_lowercase().contains(&needle))
        })
        .collect()
}

fn filtered_k8s<'a>(snapshot: &'a Snapshot, filter: &str) -> Vec<&'a crate::models::K8sBackupRow> {
    let needle = filter.to_lowercase();
    snapshot
        .k8s_backups
        .iter()
        .filter(|row| {
            needle.is_empty()
                || [
                    row.name.as_str(),
                    row.namespace.as_str(),
                    row.source_type.as_str(),
                    row.status.as_str(),
                ]
                .iter()
                .any(|value| value.to_lowercase().contains(&needle))
        })
        .collect()
}

fn centered_rect(area: Rect, width_percent: u16, height_percent: u16) -> Rect {
    let vertical = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Percentage((100 - height_percent) / 2),
            Constraint::Percentage(height_percent),
            Constraint::Percentage((100 - height_percent) / 2),
        ])
        .split(area);
    Layout::default()
        .direction(Direction::Horizontal)
        .constraints([
            Constraint::Percentage((100 - width_percent) / 2),
            Constraint::Percentage(width_percent),
            Constraint::Percentage((100 - width_percent) / 2),
        ])
        .split(vertical[1])[1]
}
