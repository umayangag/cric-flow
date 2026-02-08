# Context Provider MCP Server

This tool indexes the codebase to provide context for AI agents.
It implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/).

## Features
- **Smart Indexing**: Scans the project directory (respecting ignores).
- **Symbol Extraction**: Parses Go and Python files to extract symbols (functions, classes).
- **Persistence**: Saves the index to `.junie/context_index.json` to avoid re-scanning on every start.
- **MCP Support**:
    - Tools: `get_summary`, `refresh_index`.
    - Resources: `context://summary`.

## Usage

### Build
```bash
make context-build
# OR
cd context-provider && go build -o context-provider main.go
```

### Run
Run as an MCP server (stdio transport):
```bash
./context-provider/context-provider
```

### Configuration
The server automatically detects the project root. You can explicitly set it with `-root`.

## Persistence
The context index is saved to `<PROJECT_ROOT>/.junie/context_index.json`.
- **On Start**: The server tries to load this file. If found, it starts instantly. If not, it performs a full scan.
- **Refresh**: Calling the `refresh_index` tool forces a re-scan and updates the file.

## Integration with IDEs (e.g., JetBrains for Junie)

To add this tool permanently to Junie/JetBrains:

1.  **Build the Tool**:
    ```bash
    make context-build
    ```

2.  **Open Settings**: Go to `Settings` > `Tools` > `Model Context Protocol`.

3.  **Add Server**: Add a new server definition.

4.  **JSON Configuration**:
    ```json
    {
      "cric-info-context": {
        "command": "<PROJECT_ROOT>/context-provider/context-provider",
        "args": [
          "-root",
          "<PROJECT_ROOT>"
        ]
      }
    }
    ```
    *Replace `<PROJECT_ROOT>` with the absolute path to your repo.*

### How to Prompt Junie
- "Run the context-provider to get a summary of the project."
- "Refresh the context index."
