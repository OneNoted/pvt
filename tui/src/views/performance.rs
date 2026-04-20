use crate::models::{PodMetricRow, Snapshot};
use crate::util::truncate;
use crossterm::event::{KeyCode, KeyEvent};
use ratatui::{
    Frame,
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Cell, Paragraph, Row, Table, TableState},
};
use std::cmp::Ordering;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SortColumn {
    Pod,
    Namespace,
    Cpu,
    Memory,
    NetRx,
    NetTx,
}

pub struct PerformanceViewState {
    pub table: TableState,
    pub sort_col: SortColumn,
    pub sort_asc: bool,
    pub ns_index: usize,
}

impl Default for PerformanceViewState {
    fn default() -> Self {
        Self {
            table: TableState::default(),
            sort_col: SortColumn::Cpu,
            sort_asc: false,
            ns_index: 0,
        }
    }
}

impl PerformanceViewState {
    pub fn handle_key(&mut self, key: KeyEvent, pod_count: usize, namespace_count: usize) {
        match key.code {
            KeyCode::Char('s') => {
                self.sort_col = match self.sort_col {
                    SortColumn::Pod => SortColumn::Namespace,
                    SortColumn::Namespace => SortColumn::Cpu,
                    SortColumn::Cpu => SortColumn::Memory,
                    SortColumn::Memory => SortColumn::NetRx,
                    SortColumn::NetRx => SortColumn::NetTx,
                    SortColumn::NetTx => SortColumn::Pod,
                }
            }
            KeyCode::Char('S') => self.sort_asc = !self.sort_asc,
            KeyCode::Char('n') => {
                self.ns_index = if namespace_count == 0 {
                    0
                } else {
                    (self.ns_index + 1) % (namespace_count + 1)
                };
            }
            KeyCode::Char('j') | KeyCode::Down => {
                if pod_count > 0 {
                    let next = self
                        .table
                        .selected()
                        .unwrap_or(0)
                        .saturating_add(1)
                        .min(pod_count - 1);
                    self.table.select(Some(next));
                }
            }
            KeyCode::Char('k') | KeyCode::Up => {
                let next = self.table.selected().unwrap_or(0).saturating_sub(1);
                self.table.select(Some(next));
            }
            KeyCode::Char('g') => self.table.select(Some(0)),
            KeyCode::Char('G') => {
                if pod_count > 0 {
                    self.table.select(Some(pod_count - 1));
                }
            }
            _ => {}
        }
    }
}

pub fn namespace_count(snapshot: &Snapshot) -> usize {
    unique_namespaces(snapshot).len()
}

pub fn visible_pod_count(snapshot: &Snapshot, ns_index: usize) -> usize {
    let namespaces = unique_namespaces(snapshot);
    let active_ns = if ns_index == 0 {
        None
    } else {
        namespaces.get(ns_index - 1).map(String::as_str)
    };
    snapshot
        .pods
        .iter()
        .filter(|pod| active_ns.map(|ns| pod.namespace == ns).unwrap_or(true))
        .count()
}

pub fn render(
    frame: &mut Frame,
    area: Rect,
    state: &mut PerformanceViewState,
    snapshot: &Snapshot,
) {
    if !snapshot.metrics_available && snapshot.hosts.is_empty() {
        frame.render_widget(
            Paragraph::new("No metrics backend detected")
                .block(Block::default().borders(Borders::ALL).title("Performance")),
            area,
        );
        return;
    }

    let namespaces = unique_namespaces(snapshot);
    if state.ns_index > namespaces.len() {
        state.ns_index = 0;
    }
    let active_ns = if state.ns_index == 0 {
        None
    } else {
        Some(namespaces[state.ns_index - 1].as_str())
    };

    let mut pods = snapshot
        .pods
        .iter()
        .filter(|pod| active_ns.map(|ns| pod.namespace == ns).unwrap_or(true))
        .cloned()
        .collect::<Vec<_>>();
    sort_pods(&mut pods, state.sort_col, state.sort_asc);
    if pods.is_empty() {
        state.table.select(None);
    } else {
        let selected = state.table.selected().unwrap_or(0).min(pods.len() - 1);
        state.table.select(Some(selected));
    }

    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Percentage(35),
            Constraint::Percentage(60),
            Constraint::Length(2),
        ])
        .split(area);

    let host_rows = snapshot.hosts.iter().map(|host| {
        Row::new(vec![
            Cell::from(truncate(&host.name, 22)),
            Cell::from(format!("{:>5.1}%", host.cpu_pct)),
            Cell::from(format!("{} / {}", host.mem_used_str, host.mem_total_str)),
            Cell::from(format!("{:>5.1}%", host.mem_pct)),
        ])
    });
    let host_table = Table::new(
        host_rows,
        [
            Constraint::Length(22),
            Constraint::Length(8),
            Constraint::Length(24),
            Constraint::Min(8),
        ],
    )
    .header(
        Row::new(vec!["Host", "CPU", "Memory", "Mem %"]).style(
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        ),
    )
    .block(
        Block::default()
            .borders(Borders::ALL)
            .title("Host Overview"),
    );
    frame.render_widget(host_table, chunks[0]);

    let pod_rows = pods.iter().map(|pod| {
        Row::new(vec![
            Cell::from(truncate(&pod.pod, 20)),
            Cell::from(truncate(&pod.namespace, 14)),
            Cell::from(pod.cpu_str.clone()),
            Cell::from(pod.mem_str.clone()),
            Cell::from(pod.net_rx_str.clone()),
            Cell::from(pod.net_tx_str.clone()),
        ])
    });
    let title = format!("Pod Metrics [ns: {}]", active_ns.unwrap_or("all"));
    let pod_table = Table::new(
        pod_rows,
        [
            Constraint::Length(20),
            Constraint::Length(14),
            Constraint::Length(10),
            Constraint::Length(12),
            Constraint::Length(12),
            Constraint::Min(12),
        ],
    )
    .header(
        Row::new(vec![
            "Pod",
            "Namespace",
            "CPU",
            "Memory",
            "Net RX",
            "Net TX",
        ])
        .style(
            Style::default()
                .fg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        ),
    )
    .block(Block::default().borders(Borders::ALL).title(title))
    .row_highlight_style(
        Style::default()
            .bg(Color::Blue)
            .fg(Color::Black)
            .add_modifier(Modifier::BOLD),
    );
    if state.table.selected().is_none() && !pods.is_empty() {
        state.table.select(Some(0));
    }
    frame.render_stateful_widget(pod_table, chunks[1], &mut state.table);

    let hint = Line::from(vec![
        Span::styled(
            "s/S",
            Style::default()
                .fg(Color::Yellow)
                .add_modifier(Modifier::BOLD),
        ),
        Span::raw(" sort  "),
        Span::styled(
            "n",
            Style::default()
                .fg(Color::Yellow)
                .add_modifier(Modifier::BOLD),
        ),
        Span::raw(" namespace  "),
        Span::raw(format!(
            "current sort={}{}",
            sort_name(state.sort_col),
            if state.sort_asc { " asc" } else { " desc" }
        )),
    ]);
    frame.render_widget(
        Paragraph::new(hint).block(Block::default().borders(Borders::ALL).title("Performance")),
        chunks[2],
    );
}

fn unique_namespaces(snapshot: &Snapshot) -> Vec<String> {
    let mut namespaces = snapshot
        .pods
        .iter()
        .map(|pod| pod.namespace.clone())
        .collect::<Vec<_>>();
    namespaces.sort();
    namespaces.dedup();
    namespaces
}

fn sort_pods(pods: &mut [PodMetricRow], sort_col: SortColumn, sort_asc: bool) {
    pods.sort_by(|a, b| {
        let ordering = match sort_col {
            SortColumn::Pod => a.pod.cmp(&b.pod),
            SortColumn::Namespace => a.namespace.cmp(&b.namespace),
            SortColumn::Cpu => a
                .cpu_cores
                .partial_cmp(&b.cpu_cores)
                .unwrap_or(Ordering::Equal),
            SortColumn::Memory => a
                .mem_bytes
                .partial_cmp(&b.mem_bytes)
                .unwrap_or(Ordering::Equal),
            SortColumn::NetRx => a
                .net_rx_bytes_sec
                .partial_cmp(&b.net_rx_bytes_sec)
                .unwrap_or(Ordering::Equal),
            SortColumn::NetTx => a
                .net_tx_bytes_sec
                .partial_cmp(&b.net_tx_bytes_sec)
                .unwrap_or(Ordering::Equal),
        };
        if sort_asc {
            ordering
        } else {
            ordering.reverse()
        }
    });
}

fn sort_name(column: SortColumn) -> &'static str {
    match column {
        SortColumn::Pod => "pod",
        SortColumn::Namespace => "namespace",
        SortColumn::Cpu => "cpu",
        SortColumn::Memory => "memory",
        SortColumn::NetRx => "net-rx",
        SortColumn::NetTx => "net-tx",
    }
}
