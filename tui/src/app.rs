use crate::config::Config;
use crate::models::Snapshot;
use crate::poller::{PollerCommand, PollerHandle};
use crate::views::{backups, cluster, performance, storage};
use crossterm::event::{self, Event, KeyCode, KeyEventKind};
use ratatui::{
    DefaultTerminal, Frame,
    layout::{Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, Paragraph, Tabs, Wrap},
};
use std::time::Duration;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ActiveView {
    Cluster,
    Storage,
    Backups,
    Performance,
}

impl ActiveView {
    fn index(self) -> usize {
        match self {
            Self::Cluster => 0,
            Self::Storage => 1,
            Self::Backups => 2,
            Self::Performance => 3,
        }
    }

    fn next(self) -> Self {
        match self {
            Self::Cluster => Self::Storage,
            Self::Storage => Self::Backups,
            Self::Backups => Self::Performance,
            Self::Performance => Self::Cluster,
        }
    }

    fn prev(self) -> Self {
        match self {
            Self::Cluster => Self::Performance,
            Self::Storage => Self::Cluster,
            Self::Backups => Self::Storage,
            Self::Performance => Self::Backups,
        }
    }
}

pub struct App {
    config: Config,
    snapshot: Snapshot,
    active_view: ActiveView,
    show_help: bool,
    should_quit: bool,
    cluster_state: cluster::ClusterViewState,
    storage_state: storage::StorageViewState,
    backups_state: backups::BackupsViewState,
    performance_state: performance::PerformanceViewState,
    poller: PollerHandle,
}

impl App {
    pub fn new(config: Config) -> Self {
        let poller = PollerHandle::spawn(config.clone());
        Self {
            config,
            snapshot: Snapshot {
                loading: true,
                ..Snapshot::default()
            },
            active_view: ActiveView::Cluster,
            show_help: false,
            should_quit: false,
            cluster_state: cluster::ClusterViewState::default(),
            storage_state: storage::StorageViewState::default(),
            backups_state: backups::BackupsViewState::default(),
            performance_state: performance::PerformanceViewState::default(),
            poller,
        }
    }

    pub fn run(mut self, terminal: &mut DefaultTerminal) -> anyhow::Result<()> {
        while !self.should_quit {
            while let Ok(snapshot) = self.poller.snapshots.try_recv() {
                self.snapshot = snapshot;
            }

            terminal.draw(|frame| self.draw(frame))?;

            if event::poll(Duration::from_millis(100))?
                && let Event::Key(key) = event::read()?
                && key.kind == KeyEventKind::Press
            {
                self.handle_key(key);
            }
        }
        Ok(())
    }

    fn handle_key(&mut self, key: event::KeyEvent) {
        if self.show_help {
            match key.code {
                KeyCode::Esc | KeyCode::Char('?') => self.show_help = false,
                _ => {}
            }
            return;
        }

        match key.code {
            KeyCode::Char('q') => {
                self.should_quit = true;
                return;
            }
            KeyCode::Char('?') => {
                self.show_help = true;
                return;
            }
            KeyCode::Char('r') => {
                let _ = self.poller.commands.send(PollerCommand::RefreshNow);
                return;
            }
            KeyCode::Char('1') => {
                self.active_view = ActiveView::Cluster;
                return;
            }
            KeyCode::Char('2') => {
                self.active_view = ActiveView::Storage;
                return;
            }
            KeyCode::Char('3') => {
                self.active_view = ActiveView::Backups;
                return;
            }
            KeyCode::Char('4') => {
                self.active_view = ActiveView::Performance;
                return;
            }
            KeyCode::Tab => {
                self.active_view = self.active_view.next();
                return;
            }
            KeyCode::BackTab => {
                self.active_view = self.active_view.prev();
                return;
            }
            _ => {}
        }

        match self.active_view {
            ActiveView::Cluster => self
                .cluster_state
                .handle_key(key, self.snapshot.cluster_rows.len()),
            ActiveView::Storage => self.storage_state.handle_key(
                key,
                self.snapshot.storage_pools.len(),
                self.snapshot.vm_disks.len(),
            ),
            ActiveView::Backups => {
                if let Some(action) = self.backups_state.handle_key(key, &self.snapshot) {
                    let _ = self
                        .poller
                        .commands
                        .send(PollerCommand::DeleteBackup(action));
                }
            }
            ActiveView::Performance => self.performance_state.handle_key(
                key,
                performance::visible_pod_count(&self.snapshot, self.performance_state.ns_index),
                performance::namespace_count(&self.snapshot),
            ),
        }
    }

    fn draw(&mut self, frame: &mut Frame) {
        let area = frame.area();
        let outer = Layout::default()
            .direction(Direction::Vertical)
            .constraints([
                Constraint::Length(1),
                Constraint::Min(0),
                Constraint::Length(1),
            ])
            .split(area);

        self.draw_tabs(frame, outer[0]);
        if area.width < 80 || area.height < 24 {
            frame.render_widget(
                Paragraph::new("Terminal too small (min 80x24)")
                    .block(Block::default().borders(Borders::ALL).title("vitui")),
                outer[1],
            );
        } else if self.snapshot.loading {
            frame.render_widget(
                Paragraph::new("Loading data…")
                    .block(Block::default().borders(Borders::ALL).title("vitui")),
                outer[1],
            );
        } else {
            match self.active_view {
                ActiveView::Cluster => {
                    cluster::render(frame, outer[1], &mut self.cluster_state, &self.snapshot)
                }
                ActiveView::Storage => storage::render(
                    frame,
                    outer[1],
                    &mut self.storage_state,
                    &self.snapshot,
                    self.config.tui.storage.warn_threshold,
                    self.config.tui.storage.crit_threshold,
                ),
                ActiveView::Backups => {
                    backups::render(frame, outer[1], &mut self.backups_state, &self.snapshot)
                }
                ActiveView::Performance => performance::render(
                    frame,
                    outer[1],
                    &mut self.performance_state,
                    &self.snapshot,
                ),
            }
        }
        self.draw_status(frame, outer[2]);

        if self.show_help {
            self.draw_help(frame, area);
        }
    }

    fn draw_tabs(&self, frame: &mut Frame, area: Rect) {
        let titles = ["1:Cluster", "2:Storage", "3:Backups", "4:Perf"]
            .into_iter()
            .map(Line::from)
            .collect::<Vec<_>>();
        let tabs = Tabs::new(titles)
            .select(self.active_view.index())
            .style(Style::default().fg(Color::White).bg(Color::DarkGray))
            .highlight_style(
                Style::default()
                    .fg(Color::Black)
                    .bg(Color::Blue)
                    .add_modifier(Modifier::BOLD),
            );
        frame.render_widget(tabs, area);
    }

    fn draw_status(&self, frame: &mut Frame, area: Rect) {
        let refresh = self
            .snapshot
            .last_refresh_label
            .clone()
            .unwrap_or_else(|| "never".to_string());
        let mut spans = vec![
            Span::raw(format!(" refresh={}  ", refresh)),
            Span::styled(
                "r",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(" refresh  "),
            Span::styled(
                "?",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(" help  "),
            Span::styled(
                "q",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ),
            Span::raw(" quit"),
        ];
        if let Some(error) = &self.snapshot.last_error {
            spans.push(Span::raw("  |  "));
            spans.push(Span::styled(
                truncate_for_status(error),
                Style::default().fg(Color::Yellow),
            ));
        }
        frame.render_widget(
            Paragraph::new(Line::from(spans)).block(Block::default().borders(Borders::ALL)),
            area,
        );
    }

    fn draw_help(&self, frame: &mut Frame, area: Rect) {
        let popup = centered_rect(area, 72, 60);
        frame.render_widget(Clear, popup);
        let text = "Global keys\n\n1-4 / Tab / Shift-Tab  switch views\nr                      refresh\n?                      help\nq                      quit\n\nCluster\n  j/k, arrows, g/G     move selection\n\nStorage\n  h/l or left/right    switch between pools/disks\n  j/k, arrows, g/G     move selection\n\nBackups\n  /                    filter\n  d                    delete selected PVE backup\n  y/n                  confirm/cancel delete\n\nPerformance\n  s / S                cycle sort / reverse sort\n  n                    cycle namespace filter\n  j/k, arrows, g/G     move selection";
        frame.render_widget(
            Paragraph::new(text)
                .wrap(Wrap { trim: true })
                .block(Block::default().borders(Borders::ALL).title("Help")),
            popup,
        );
    }
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

fn truncate_for_status(value: &str) -> String {
    let max = 120;
    if value.chars().count() <= max {
        value.to_string()
    } else {
        let mut out = value.chars().take(max - 1).collect::<String>();
        out.push('…');
        out
    }
}
