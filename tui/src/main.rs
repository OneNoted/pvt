mod app;
mod config;
mod integrations;
mod models;
mod poller;
mod util;
mod views;

use anyhow::Result;
use app::App;
use config::{load_from_path, parse_args};
fn main() -> Result<()> {
    let Some(config_path) = parse_args()? else {
        return Ok(());
    };
    let config = load_from_path(&config_path)?;

    let mut terminal = ratatui::init();
    let result = App::new(config).run(&mut terminal);
    ratatui::restore();
    result
}
