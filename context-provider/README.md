# Context Provider MCP Server

This tool indexes the codebase to provide context for AI agents.
It implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/).

## Features
- Scans the project directory (respecting ignores).
- Parses Go and Python files to extract symbols (functions, classes).
- Reads config/docs content.
- Exposes tools to get a summary or refresh the index.
- Exposes a resource `context://summary` with the full structured context.

## Usage

### Build
```bash
go build -o context-provider main.go
```

### Run
Run as an MCP server (stdio transport):
```bash
./context-provider
```

### Configuration
The server automatically detects the project root (either current directory or parent if running from `context-provider` dir).

## MCP Capabilities

### Tools
- `get_summary`: Returns a high-level text summary.
- `refresh_index`: Re-scans the codebase.

### Resources
- `context://summary`: JSON representation of the full codebase index.
