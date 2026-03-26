---
name: example-skill
description: An example skill demonstrating DeerFlow-aligned metadata format
version: 1.0.0
author: memoh-team
license: MIT
allowed-tools:
  - web_search
  - web_fetch
  - read_file
  - write_file
compatibility: memoh>=2.0
category: productivity
---

# Example Skill

This is an example skill that demonstrates the DeerFlow-aligned metadata format for Memoh.

## When to Use

Use this skill when you want to:
- Demonstrate the new skill format
- Test the skill installation system
- Understand the metadata options

## Usage

The skill system will automatically:
1. Parse the YAML frontmatter
2. Validate the skill structure
3. Apply tool restrictions (if allowed-tools is specified)
4. Inject the content into the system prompt when activated

## Tool Restrictions

This skill is restricted to using only these tools:
- `web_search` - For searching the web
- `web_fetch` - For fetching web content
- `read_file` - For reading files
- `write_file` - For writing files

Any attempt to use other tools will be blocked.

## Metadata Fields

| Field | Required | Description |
|-------|----------|-------------|
| name | Yes | Skill identifier (hyphen-case) |
| description | Yes | Short description (max 1024 chars) |
| version | No | Semantic version (e.g., 1.0.0) |
| author | No | Author or organization |
| license | No | License identifier (e.g., MIT) |
| allowed-tools | No | List of permitted tools |
| compatibility | No | Version compatibility (e.g., memoh>=2.0) |
| category | No | Category for grouping |
