//! `a11y-cli` is a small AT-SPI2 helper invoked by the Memoh workspace
//! Computer Use tools. It probes the accessibility bus, walks the desktop
//! accessibility tree to produce a flat ref list (`e1..eN`), and performs
//! ref-based click/type/fill actions on the focused desktop. When an action
//! cannot be performed through AT-SPI, the binary returns a `fallback`
//! coordinate that the Go caller will replay over RFB.

use anyhow::Result;
use clap::{Parser, Subcommand};

mod action;
mod connection;
mod probe;
mod refs;
mod snapshot;

/// Top-level CLI definition.
#[derive(Parser)]
#[command(name = "a11y-cli", about = "Memoh workspace AT-SPI2 helper", version)]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Connect to the accessibility bus and report status as JSON.
    Probe,
    /// Walk the desktop accessibility tree and emit a flat snapshot.
    Snapshot {
        /// Hard cap on the number of interactive nodes returned.
        #[arg(long, default_value_t = 300)]
        limit: usize,
    },
    /// Invoke the default action (typically "click") on a ref.
    Click {
        #[arg(long)]
        r#ref: String,
    },
    /// Insert text into the editable element backing a ref.
    Type {
        #[arg(long)]
        r#ref: String,
        #[arg(long)]
        text: String,
    },
    /// Replace the editable contents of a ref.
    Fill {
        #[arg(long)]
        r#ref: String,
        #[arg(long)]
        text: String,
    },
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    futures_lite::future::block_on(async move {
        match cli.command {
            Command::Probe => probe::run().await,
            Command::Snapshot { limit } => snapshot::run(limit).await,
            Command::Click { r#ref } => action::click(&r#ref).await,
            Command::Type { r#ref, text } => action::type_text(&r#ref, &text).await,
            Command::Fill { r#ref, text } => action::fill_text(&r#ref, &text).await,
        }
    })
}
