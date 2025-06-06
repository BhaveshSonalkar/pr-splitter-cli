# PR Splitter CLI Plugins

This directory contains language-specific dependency analyzer plugins for the PR Splitter CLI. Plugins are automatically discovered and loaded based on their manifest files.

## Plugin Structure

Each plugin must be in its own directory with the following structure:

```
plugins/
├── your-plugin/
│   ├── plugin.json        # Plugin manifest (required)
│   ├── analyzer.*         # Main executable
│   └── ... other files
```

## Plugin Manifest (plugin.json)

Each plugin directory must contain a `plugin.json` manifest file with the following structure:

```json
{
  "name": "Your Plugin Name",
  "executable": "analyzer.js",
  "extensions": [".ext1", ".ext2"],
  "description": "Brief description of what the plugin does",
  "version": "1.0.0",
  "runtime": "node",
  "author": "Your Name",
  "homepage": "https://github.com/your-org/your-plugin"
}
```

### Manifest Fields

- **name** (required): Human-readable name of the plugin
- **executable** (required): Path to the main executable file (relative to plugin directory)
- **extensions** (required): Array of file extensions this plugin handles (e.g., [".ts", ".tsx"])
- **description** (required): Brief description of the plugin's functionality
- **version** (required): Semantic version of the plugin
- **runtime** (optional): Runtime needed to execute the plugin (`node`, `python3`, etc.)
- **author** (optional): Plugin author information
- **homepage** (optional): URL to plugin documentation or repository

## Runtime Detection

If `runtime` is not specified, the system will attempt to detect it based on the executable file extension:

- `.js` files → `node`
- `.py` files → `python3`
- Other extensions → Executed directly as binary

## Plugin Interface

### Input

Plugins receive JSON input via stdin with the following structure:

```json
{
  "changedFiles": [
    {
      "path": "src/file.ts",
      "changeType": "MODIFY",
      "content": "file content...",
      "linesAdded": 10,
      "linesDeleted": 5,
      "isChanged": true
    }
  ],
  "projectFiles": [
    {
      "path": "src/other.ts",
      "content": "other file content...",
      "isChanged": false
    }
  ],
  "projectRoot": "/path/to/project"
}
```

### Output

Plugins must output JSON to stdout with the following structure:

```json
{
  "dependencies": [
    {
      "from": "src/file.ts",
      "to": "src/other.ts",
      "type": "import",
      "strength": "CRITICAL",
      "line": 1,
      "context": "import { something } from './other'"
    }
  ],
  "metadata": {
    "filesAnalyzed": 5,
    "analysisTime": "150ms",
    "pluginName": "your-plugin",
    "pluginVersion": "1.0.0"
  },
  "errors": []
}
```

#### Dependency Strength Levels

- **CRITICAL**: Import/export statements that break compilation if removed
- **STRONG**: Function calls, require() statements that break runtime
- **MODERATE**: Type references, dynamic imports that break features
- **WEAK**: Similar patterns, conventions that reduce code quality
- **CIRCULAR**: Mutual dependencies between files

## Available Plugins

### TypeScript/JavaScript Plugin
- **Directory**: `typescript/`
- **Extensions**: `.ts`, `.tsx`, `.js`, `.jsx`
- **Runtime**: Node.js
- **Features**: AST-based analysis using TypeScript Compiler API

### Python Plugin
- **Directory**: `python/`
- **Extensions**: `.py`, `.pyi`
- **Runtime**: Python 3
- **Features**: AST-based analysis of import statements

## Creating a New Plugin

1. Create a new directory in `plugins/` with your plugin name
2. Create a `plugin.json` manifest file with required fields
3. Implement your analyzer following the input/output interface
4. Test your plugin by running the PR Splitter CLI

### Example Plugin Template

```javascript
#!/usr/bin/env node

// Read input
let input = '';
process.stdin.on('data', chunk => input += chunk);
process.stdin.on('end', () => {
  const data = JSON.parse(input);
  
  // Analyze dependencies
  const dependencies = analyzeFiles(data.changedFiles, data.projectFiles);
  
  // Output result
  console.log(JSON.stringify({
    dependencies: dependencies,
    metadata: {
      filesAnalyzed: data.changedFiles.length,
      analysisTime: "50ms",
      pluginName: "my-plugin",
      pluginVersion: "1.0.0"
    },
    errors: []
  }));
});

function analyzeFiles(changed, project) {
  // Your analysis logic here
  return [];
}
```

## Plugin Discovery

The system automatically discovers plugins by:

1. Scanning the `plugins/` directory for subdirectories
2. Looking for `plugin.json` manifest in each subdirectory
3. Validating the manifest and checking if the executable exists
4. Verifying required runtime is available (if specified)
5. Registering valid plugins for use

Plugins that fail validation are logged but don't prevent other plugins from loading. 