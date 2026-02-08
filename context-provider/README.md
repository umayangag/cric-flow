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

## Integration with IDEs (e.g., JetBrains for Junie)

This tool is designed to help AI agents (like Junie) efficiently understand the project context without re-reading every file.

### Setup for User
1.  **Build the Tool**:
    Ensure the `context-provider` binary is built.
    ```bash
    make context-serve
    # OR manually:
    cd context-provider && go build -o context-provider main.go
    ```

### How to Prompt Junie
When working with Junie in your IDE, you can instruct it to use this tool to "get up to speed" on the project.

**Sample Prompts:**
- > "Run the context-provider to get a summary of the project structure."
- > "Use the `context-provider` tool to list all symbols in `ml-service`."
- > "I have a local context server running. Please query it for the project summary."

**How Junie uses it (Internal):**
Junie can execute the tool via shell to perform JSON-RPC requests:
```bash
# Requesting a summary
echo '{"jsonrpc": "2.0", "method": "get_summary", "id": 1}' | ./context-provider/context-provider
```
The tool responds with a high-level summary of files, types, and stats, which Junie then uses to answer your questions accurately.
