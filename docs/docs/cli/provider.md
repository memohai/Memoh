# Provider Commands

Manage LLM providers (API endpoints and credentials).

## provider list

List all providers. Optionally filter by provider name.

```bash
memoh provider list [options]
```

| Option | Description |
|--------|-------------|
| `--provider <name>` | Filter by provider name |

Examples:

```bash
memoh provider list
memoh provider list --provider my-openai
```

## provider create

Create a new provider. Prompts for any missing fields.

```bash
memoh provider create [options]
```

| Option | Description |
|--------|-------------|
| `--name <name>` | Provider name |
| `--base_url <url>` | Base URL for the API |
| `--api_key <key>` | API key |

Examples:

```bash
memoh provider create --name my-ollama --base_url http://localhost:11434/v1
memoh provider create
# Interactive prompts
```

## provider delete

Delete a provider by name.

```bash
memoh provider delete --provider <name>
```

Example:

```bash
memoh provider delete --provider my-ollama
```
