//! Implements `click`, `type`, and `fill` actions in terms of AT-SPI. Whenever
//! the AT-SPI invocation fails we still emit `fallback: { x, y }` so the Go
//! caller can replay the action via RFB pointer/key events.

use anyhow::Result;
use serde::Serialize;

use crate::connection;
use crate::refs::{self, RefEntry};

#[derive(Serialize)]
struct ActionResult {
    ok: bool,
    action: &'static str,
    #[serde(rename = "ref")]
    ref_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    detail: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    fallback: Option<Fallback>,
}

#[derive(Serialize)]
struct Fallback {
    x: i32,
    y: i32,
}

impl ActionResult {
    fn success(action: &'static str, entry: &RefEntry, detail: impl Into<String>) -> Self {
        Self {
            ok: true,
            action,
            ref_id: entry.ref_id.clone(),
            detail: Some(detail.into()),
            error: None,
            fallback: None,
        }
    }

    fn failure(action: &'static str, entry: &RefEntry, error: impl Into<String>) -> Self {
        let (x, y) = entry.center();
        Self {
            ok: false,
            action,
            ref_id: entry.ref_id.clone(),
            detail: None,
            error: Some(error.into()),
            fallback: Some(Fallback { x, y }),
        }
    }

    fn emit(&self) -> Result<()> {
        println!("{}", serde_json::to_string(self)?);
        Ok(())
    }
}

pub async fn click(ref_id: &str) -> Result<()> {
    let entry = refs::lookup(ref_id)?;
    let outcome = try_click(&entry).await;
    match outcome {
        Ok(detail) => ActionResult::success("click", &entry, detail).emit(),
        Err(err) => ActionResult::failure("click", &entry, format!("{err:#}")).emit(),
    }
}

async fn try_click(entry: &RefEntry) -> Result<String> {
    let conn = connection::open().await?;
    let object = entry.to_object_ref()?;
    let accessible = connection::accessible_for(&conn, &object).await?;
    let actions = connection::action_for(&conn, &accessible).await?;

    let descriptors = actions.get_actions().await?;
    if descriptors.is_empty() {
        anyhow::bail!("the target element does not expose any AT-SPI actions");
    }
    let preferred = preferred_action_index(&descriptors);
    let success = actions.do_action(preferred as i32).await?;
    if !success {
        anyhow::bail!("AT-SPI reported the action did not run");
    }
    let label = descriptors
        .get(preferred)
        .map(|action| action.name.to_string())
        .unwrap_or_else(|| "click".to_string());
    Ok(label)
}

fn preferred_action_index(descriptors: &[atspi::Action]) -> usize {
    for (idx, action) in descriptors.iter().enumerate() {
        let lower = action.name.to_ascii_lowercase();
        if lower.contains("click") || lower.contains("press") || lower.contains("activate") {
            return idx;
        }
    }
    0
}

/// The `length` argument of AT-SPI `EditableText.InsertText` is interpreted as
/// the number of **UTF-8 bytes** by the toolkits we drive (GTK/ATK documents it
/// as "length ... in bytes", and Chromium copies `length` bytes out of the
/// string). Passing the Unicode scalar count instead truncates multi-byte text
/// such as CJK — e.g. "你好" is 2 chars but 6 bytes, so a length of 2 inserts a
/// broken prefix. Always derive the length from the UTF-8 byte length.
fn insert_text_length(text: &str) -> i32 {
    i32::try_from(text.len()).unwrap_or(i32::MAX)
}

pub async fn type_text(ref_id: &str, text: &str) -> Result<()> {
    let entry = refs::lookup(ref_id)?;
    let outcome = try_type(&entry, text).await;
    match outcome {
        Ok(_) => ActionResult::success(
            "type",
            &entry,
            format!("inserted {} chars", text.chars().count()),
        )
        .emit(),
        Err(err) => ActionResult::failure("type", &entry, format!("{err:#}")).emit(),
    }
}

async fn try_type(entry: &RefEntry, text: &str) -> Result<()> {
    let conn = connection::open().await?;
    let object = entry.to_object_ref()?;
    let accessible = connection::accessible_for(&conn, &object).await?;
    let editable = connection::editable_for(&conn, &accessible).await?;
    let text_proxy = connection::text_for(&conn, &accessible).await?;
    let caret = text_proxy.caret_offset().await.unwrap_or(-1);
    let position = if caret < 0 { 0 } else { caret };
    let length = insert_text_length(text);
    let inserted = editable.insert_text(position, text, length).await?;
    if !inserted {
        anyhow::bail!("editable text widget refused to insert");
    }
    Ok(())
}

pub async fn fill_text(ref_id: &str, text: &str) -> Result<()> {
    let entry = refs::lookup(ref_id)?;
    let outcome = try_fill(&entry, text).await;
    match outcome {
        Ok(_) => ActionResult::success(
            "fill",
            &entry,
            format!("set {} chars", text.chars().count()),
        )
        .emit(),
        Err(err) => ActionResult::failure("fill", &entry, format!("{err:#}")).emit(),
    }
}

async fn try_fill(entry: &RefEntry, text: &str) -> Result<()> {
    let conn = connection::open().await?;
    let object = entry.to_object_ref()?;
    let accessible = connection::accessible_for(&conn, &object).await?;
    let editable = connection::editable_for(&conn, &accessible).await?;
    let replaced = editable.set_text_contents(text).await?;
    if !replaced {
        anyhow::bail!("editable text widget refused to replace contents");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn action(name: &str) -> atspi::Action {
        atspi::Action {
            name: name.to_string(),
            description: String::new(),
            keybinding: String::new(),
        }
    }

    #[test]
    fn preferred_index_picks_click_first() {
        let descriptors = [action("focus"), action("click"), action("press")];
        assert_eq!(preferred_action_index(&descriptors), 1);
    }

    #[test]
    fn preferred_index_matches_press_when_no_click() {
        let descriptors = [action("focus"), action("press"), action("activate")];
        assert_eq!(preferred_action_index(&descriptors), 1);
    }

    #[test]
    fn preferred_index_matches_activate_when_no_click_or_press() {
        let descriptors = [action("focus"), action("activate")];
        assert_eq!(preferred_action_index(&descriptors), 1);
    }

    #[test]
    fn preferred_index_is_case_insensitive() {
        let descriptors = [action("Focus"), action("CLICK")];
        assert_eq!(preferred_action_index(&descriptors), 1);
    }

    #[test]
    fn preferred_index_falls_back_to_zero() {
        let descriptors = [action("focus"), action("describe")];
        assert_eq!(preferred_action_index(&descriptors), 0);
    }

    #[test]
    fn preferred_index_handles_empty_descriptors() {
        let descriptors: [atspi::Action; 0] = [];
        assert_eq!(preferred_action_index(&descriptors), 0);
    }

    #[test]
    fn insert_text_length_uses_utf8_byte_count() {
        // ASCII: byte length equals char count.
        assert_eq!(insert_text_length("abc"), 3);
        // CJK: each char is 3 UTF-8 bytes, so the length must be 6, not 2.
        assert_eq!(insert_text_length("你好"), 6);
        // Mixed content keeps byte semantics.
        assert_eq!(insert_text_length("a你b"), 5);
        // Astral plane (emoji) is 4 bytes.
        assert_eq!(insert_text_length("😀"), 4);
        assert_eq!(insert_text_length(""), 0);
    }
}
