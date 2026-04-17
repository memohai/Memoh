# Bot Skills

Skills are reusable prompt modules that extend a bot's behavior, style, and tool-usage guidance. You manage them from the bot's **Skills** tab, and you can either write them yourself or install them from **[Supermarket](/getting-started/supermarket.md)**.

---

## What A Skill Looks Like

A skill is a Markdown file named `SKILL.md` with YAML frontmatter. At minimum, give it a stable `name` and a short `description`.

```yaml
---
name: coder-skill
description: Enables advanced coding workflows and tool usage.
---

# Coder Skill

You write clear code, explain trade-offs, and use file or command tools when they help complete the task.
```

Practical rules:

- Use a simple ASCII skill name such as `coder-skill`, `research`, or `docs-helper`.
- Avoid spaces in the `name`; Memoh uses it as the skill directory name.
- The Markdown body is the actual instruction content injected into the bot runtime.

---

## Where Skills Come From

Memoh distinguishes between **managed** skills and **discovered** skills:

- **Managed** skills are the ones you create, edit, or install through Memoh. They live under `/data/skills/<name>/SKILL.md`.
- **Discovered** skills are found from compatibility locations inside the bot environment, such as legacy skill directories from imported images or older setups.

Memoh scans these container-internal roots in order:

| Type | Root |
|------|------|
| Managed | `/data/skills/` |
| Legacy discovered | `/data/.skills/` |
| Compatibility discovered | `/data/.agents/skills/` |
| Compatibility discovered | `/root/.agents/skills/` |

Within each root, Memoh looks for `SKILL.md` either directly under the root or inside a named subdirectory such as `/data/skills/coder-skill/SKILL.md`.

This matters because the same skill name can appear from multiple sources. Memoh resolves those duplicates into states.

---

## Skill States

Each listed skill source has one of these states:

| State | Meaning |
|-------|---------|
| `effective` | This is the version currently active for that skill name |
| `shadowed` | Another source with the same skill name takes precedence |
| `disabled` | This specific source has been disabled and will not be used |

The important mental model is: **the skill name is the identity**. If Memoh finds multiple `coder-skill` sources, only one can be `effective`.

### Typical Examples

- A skill you just created in Memoh is usually `managed` + `effective`.
- A legacy skill can show up as `effective` until you create or adopt a managed skill with the same name.
- After you adopt a discovered skill into the managed directory, the managed copy becomes `effective` and the old source usually becomes `shadowed`.
- If you disable the effective source, another source with the same name may become `effective` automatically.

---

## Managing Skills In The UI

Open a bot, then go to **Skills**.

### Add Skill

1. Click **Add Skill**.
2. Fill in the raw Markdown in the editor.
3. Save it.

Memoh writes the file into its managed skills directory.

### Edit Skill

- Use **Edit** on a card to update the raw `SKILL.md` content.
- Editing is most useful for managed skills you own directly in Memoh.

### Delete Skill

- **Delete** removes the managed skill directory for that skill name.
- Deleting a managed skill can expose a discovered fallback source with the same name, making that fallback become `effective`.

### Disable / Enable

- **Disable** turns off one specific skill source without deleting it.
- **Enable** re-enables a previously disabled source.

Use this when you want to test a fallback or temporarily remove a skill from the prompt without losing its content.

### Adopt

**Adopt** copies a discovered skill into Memoh's managed skills directory so you can own and edit it from the UI.

Use adopt when:

- a skill came from a legacy or compatibility source
- you want that skill to become part of your Memoh-managed configuration
- you want to edit it safely in the UI later

Adopt is not available once a managed skill with the same name already exists.

---

## Effective Skills At Runtime

Only **effective** skills are loaded into the bot runtime.

That means:

- `shadowed` skills are visible for inspection, but not used
- `disabled` skills are ignored
- the active prompt only sees the current effective set

In active sessions, the **Session Status Panel** can also show which skills were used during that session.

---

## Supermarket And Imported Skills

Two common ways skills appear without being typed manually:

- **Supermarket install**: Memoh downloads the selected skill into the managed skills directory, so it behaves like a normal managed skill.
- **Imported / legacy environment**: Memoh discovers existing skills from compatibility paths and shows them as discovered sources.

If a discovered skill is useful but you want to fully manage it in Memoh, adopt it.

---

## Recommended Workflow

1. Start with a small number of focused skills.
2. Prefer clear names and short descriptions.
3. Use **Disable** before **Delete** if you are unsure.
4. Use **Adopt** for legacy skills you plan to keep.
5. Install reusable skills from **[Supermarket](/getting-started/supermarket.md)** instead of copy-pasting them repeatedly.
