# MCP Connections

Memoh fully supports the **Model Context Protocol (MCP)**, allowing you to connect your Bot to external tool services and context providers. 

## Concept: Extending Bot Capabilities

MCP provides a standardized way for bots to access external data sources and tools. Each Bot can have its own independent set of MCP connections.

---

## Connection Types

Configure MCP connections from the **MCP** tab in the Bot Detail page.

### 1. Stdio (Local Process)

This type of connection executes a local command on the server and communicates via standard input/output.

| Field | Description |
|-------|-------------|
| **Command** | The executable to run (e.g., `npx`, `python3`). |
| **Arguments** | A list of command-line arguments (e.g., `-y`, `@modelcontextprotocol/server-filesystem`, `/opt/memoh/data`). |
| **Env** | Key-value pairs for environment variables. |
| **CWD** | Working directory for the process. |

### 2. Remote (HTTP/SSE)

This type of connection connects to a remote MCP server over the network.

| Field | Description |
|-------|-------------|
| **URL** | The endpoint of the remote MCP server. |
| **Headers** | Custom HTTP headers (e.g., for authentication). |
| **Transport** | Choose between `http` or `sse` (Server-Sent Events). |

---

## Operations

- **Add Connection**: Click **Add**, choose the type, and fill in the configuration.
- **Import JSON**: Use the **Import** button to quickly add multiple connections from a standard `mcpServers` JSON config file.
- **Toggle Active**: Enable or disable specific connections without deleting them.
- **Search**: Quickly find a connection by name or ID.
- **Export**: Export a connection configuration to a JSON file.

---

## Bot Interaction

Once an MCP connection is active, the bot will automatically:
- Discover the tools and resources exposed by the MCP server.
- Use these tools to perform tasks requested by the user.
- Include the context provided by the MCP server in its reasoning process.
