# Bot Skills

Skills are the "personality" and "capabilities" of a Memoh Bot. They define how the bot should behave and what tools it can use.

## Concept: Skills as Markdown

A **Skill** is represented as a Markdown document with YAML frontmatter. These files are stored within the bot's container and parsed into tools and personality traits.

### Example Skill Structure

```yaml
---
name: coder-skill
description: Enables advanced coding capabilities and tool use.
---

# Coder Skill
As a coder, you always follow best practices and write clean, documented code. 
You can use the `edit_file` and `run_command` tools to assist the user.
```

---

## Managing Skills

Manage your bot's skill set from the **Skills** tab in the Bot Detail page.

### Adding a Skill

1. Click **Add Skill**.
2. A dialog with a basic template will open in the **Monaco Editor**.
3. Fill in the `name`, `description`, and content.
4. Click **Save**.

### Editing and Deleting

- **Edit**: Click the pencil icon next to a skill card to modify its content or frontmatter.
- **Delete**: Click the trash icon to remove a skill from the bot's container.

---

## How Bots Use Skills

- Skills are injected into the bot's system prompt during conversation.
- The YAML frontmatter helps the system categorize and manage the skills as tools.
- Modular skills allow you to easily "swap" behaviors or capabilities without rewriting the entire bot.
