#!/usr/bin/env node

/**
 * TypeScript/JavaScript Dependency Analyzer Plugin
 * 
 * Pure JavaScript implementation for analyzing TypeScript and JavaScript files
 * Compatible with Node.js without TypeScript compilation
 * 
 * Input: JSON via stdin with changed files and project context
 * Output: JSON to stdout with dependency relationships
 */

const fs = require('fs');
const path = require('path');

// Try to require TypeScript - gracefully handle if not installed
let ts;
try {
    ts = require('typescript');
} catch (err) {
    console.error(JSON.stringify({
        dependencies: [],
        errors: ['TypeScript not installed. Run: npm install typescript'],
        metadata: {
            filesAnalyzed: 0,
            analysisTime: "0ms",
            pluginName: "typescript-analyzer",
            pluginVersion: "2.0.0"
        }
    }));
    process.exit(1);
}

/**
 * TypeScript/JavaScript analyzer class
 */
class TypeScriptAnalyzer {
    constructor() {
        this.startTime = Date.now();
        this.sourceFileCache = new Map();
        this.moduleResolutionCache = new Map();
        this.errors = [];
        this.fileMap = null;
    }

    /**
     * Main analysis function
     */
    async analyze(input) {
        try {
            const changedFiles = input.changedFiles || [];
            const projectFiles = input.projectFiles || [];
            const projectRoot = input.projectRoot || process.cwd();
            
            // Filter TypeScript/JavaScript files
            const tsFiles = [...changedFiles, ...projectFiles].filter(file => 
                this.isTypeScriptFile(file.path)
            );

            if (tsFiles.length === 0) {
                return this.createResult([], 0, []);
            }

            // Create file map for efficient lookups
            this.fileMap = new Map();
            tsFiles.forEach(file => {
                this.fileMap.set(file.path, file);
                this.fileMap.set(this.normalizePath(file.path), file);
            });

            // Load TypeScript config if available
            const tsConfig = this.loadTypeScriptConfig(projectRoot);
            
            // Analyze dependencies for changed files only
            const dependencies = [];
            
            for (const file of changedFiles) {
                if (!this.isTypeScriptFile(file.path)) continue;
                
                try {
                    const fileDeps = await this.analyzeFile(file, projectRoot, tsConfig);
                    dependencies.push(...fileDeps);
                } catch (error) {
                    this.errors.push(`Error analyzing ${file.path}: ${error.message}`);
                }
            }

            // Remove duplicates and filter valid dependencies
            const uniqueDependencies = this.deduplicateDependencies(dependencies);
            const validDependencies = uniqueDependencies.filter(dep => 
                this.fileMap.has(dep.to)
            );

            return this.createResult(validDependencies, tsFiles.length, this.errors);

        } catch (error) {
            this.errors.push(`Analysis failed: ${error.message}`);
            return this.createResult([], 0, this.errors);
        }
    }

    /**
     * Check if file is a TypeScript/JavaScript file
     */
    isTypeScriptFile(filePath) {
        return /\.(ts|tsx|js|jsx|mts|cts|mjs|cjs)$/i.test(filePath);
    }

    /**
     * Check if file is a JavaScript file (for JSDoc analysis)
     */
    isJavaScriptFile(filePath) {
        return /\.(js|jsx|mjs|cjs)$/i.test(filePath);
    }

    /**
     * Load TypeScript configuration
     */
    loadTypeScriptConfig(projectRoot) {
        const configPaths = [
            path.join(projectRoot, 'tsconfig.json'),
            path.join(projectRoot, 'jsconfig.json')
        ];

        for (const configPath of configPaths) {
            try {
                if (fs.existsSync(configPath)) {
                    const configText = fs.readFileSync(configPath, 'utf8');
                    const result = ts.parseConfigFileTextToJson(configPath, configText);
                    if (!result.error) {
                        return ts.parseJsonConfigFileContent(
                            result.config,
                            ts.sys,
                            path.dirname(configPath)
                        );
                    }
                }
            } catch (error) {
                // Ignore config loading errors, use defaults
            }
        }

        // Return mixed JS/TS friendly default config
        return {
            options: {
                target: ts.ScriptTarget.ES2020,
                module: ts.ModuleKind.ESNext,
                moduleResolution: ts.ModuleResolutionKind.NodeJs,
                allowJs: true,
                checkJs: false,
                jsx: ts.JsxEmit.React,
                esModuleInterop: true,
                allowSyntheticDefaultImports: true,
                skipLibCheck: true,
                resolveJsonModule: true,
                declaration: false,
                noEmit: true
            }
        };
    }

    /**
     * Analyze a single file for dependencies
     */
    async analyzeFile(file, projectRoot, tsConfig) {
        const dependencies = [];
        
        try {
            // Create source file with proper configuration
            const sourceFile = this.getOrCreateSourceFile(file, tsConfig.options);
            if (!sourceFile) return dependencies;

            // Walk the AST to find imports and dependencies
            const visitor = (node) => {
                const dep = this.analyzeNode(node, file.path, projectRoot, tsConfig);
                if (dep) {
                    dependencies.push(dep);
                }
                ts.forEachChild(node, visitor);
            };

            visitor(sourceFile);

            // For JavaScript files, also analyze JSDoc comments for type dependencies
            if (this.isJavaScriptFile(file.path)) {
                const jsDocDeps = await this.analyzeJSDocDependencies(file, projectRoot, tsConfig);
                dependencies.push(...jsDocDeps);
            }

        } catch (error) {
            this.errors.push(`Failed to analyze ${file.path}: ${error.message}`);
        }

        return dependencies;
    }

    /**
     * Get or create TypeScript source file from cache
     */
    getOrCreateSourceFile(file, compilerOptions) {
        const cacheKey = `${file.path}-${file.content ? file.content.length : 0}`;
        
        if (this.sourceFileCache.has(cacheKey)) {
            return this.sourceFileCache.get(cacheKey);
        }

        try {
            const sourceFile = ts.createSourceFile(
                file.path,
                file.content || '',
                compilerOptions.target || ts.ScriptTarget.ES2020,
                true,
                this.getScriptKind(file.path)
            );

            this.sourceFileCache.set(cacheKey, sourceFile);
            return sourceFile;
        } catch (error) {
            return null;
        }
    }

    /**
     * Get appropriate script kind for file
     */
    getScriptKind(filePath) {
        const ext = path.extname(filePath).toLowerCase();
        switch (ext) {
            case '.ts':
            case '.mts':
            case '.cts':
                return ts.ScriptKind.TS;
            case '.tsx':
                return ts.ScriptKind.TSX;
            case '.jsx':
                return ts.ScriptKind.JSX;
            case '.mjs':
                return ts.ScriptKind.JS;
            case '.cjs':
                return ts.ScriptKind.JS;
            default:
                return ts.ScriptKind.JS;
        }
    }

    /**
     * Analyze an AST node for dependencies
     */
    analyzeNode(node, fromFile, projectRoot, tsConfig) {
        // Import declarations: import ... from "module"
        if (ts.isImportDeclaration(node)) {
            return this.analyzeImportDeclaration(node, fromFile, projectRoot, tsConfig);
        }

        // Export declarations: export ... from "module"
        if (ts.isExportDeclaration(node)) {
            return this.analyzeExportDeclaration(node, fromFile, projectRoot, tsConfig);
        }

        // Call expressions: require("module"), import("module")
        if (ts.isCallExpression(node)) {
            return this.analyzeCallExpression(node, fromFile, projectRoot, tsConfig);
        }

        // Import type/typeof expressions
        if (ts.isImportTypeNode(node)) {
            return this.analyzeImportType(node, fromFile, projectRoot, tsConfig);
        }

        return null;
    }

    /**
     * Analyze import declarations
     */
    analyzeImportDeclaration(node, fromFile, projectRoot, tsConfig) {
        if (!node.moduleSpecifier || !ts.isStringLiteral(node.moduleSpecifier)) {
            return null;
        }

        const modulePath = node.moduleSpecifier.text;
        const resolvedPath = this.resolveModulePath(modulePath, fromFile, projectRoot, tsConfig);

        if (!resolvedPath) return null;

        // Determine import strength
        let strength = 'CRITICAL';
        let importType = 'import';

        // Check for type-only imports
        if (node.importClause) {
            if (node.importClause.isTypeOnly) {
                strength = 'MODERATE';
                importType = 'type-import';
            } else if (this.isTypeOnlyImport(node.importClause)) {
                strength = 'MODERATE';
                importType = 'type-import';
            }
        }

        return {
            from: this.normalizePath(fromFile),
            to: resolvedPath,
            type: importType,
            strength: strength,
            line: this.getLineNumber(node),
            context: this.getNodeText(node).slice(0, 100)
        };
    }

    /**
     * Analyze export declarations
     */
    analyzeExportDeclaration(node, fromFile, projectRoot, tsConfig) {
        if (!node.moduleSpecifier || !ts.isStringLiteral(node.moduleSpecifier)) {
            return null;
        }

        const modulePath = node.moduleSpecifier.text;
        const resolvedPath = this.resolveModulePath(modulePath, fromFile, projectRoot, tsConfig);

        if (!resolvedPath) return null;

        return {
            from: this.normalizePath(fromFile),
            to: resolvedPath,
            type: 'export',
            strength: 'CRITICAL',
            line: this.getLineNumber(node),
            context: this.getNodeText(node).slice(0, 100)
        };
    }

    /**
     * Analyze call expressions (require, dynamic import)
     */
    analyzeCallExpression(node, fromFile, projectRoot, tsConfig) {
        // require() calls (CommonJS)
        if (ts.isIdentifier(node.expression) && node.expression.text === 'require') {
            if (node.arguments.length > 0 && ts.isStringLiteral(node.arguments[0])) {
                const modulePath = node.arguments[0].text;
                const resolvedPath = this.resolveModulePath(modulePath, fromFile, projectRoot, tsConfig);

                if (resolvedPath) {
                    return {
                        from: this.normalizePath(fromFile),
                        to: resolvedPath,
                        type: 'require',
                        strength: 'STRONG',
                        line: this.getLineNumber(node),
                        context: this.getNodeText(node).slice(0, 100)
                    };
                }
            }
        }

        // Dynamic import() calls (ES modules)
        if (node.expression.kind === ts.SyntaxKind.ImportKeyword) {
            if (node.arguments.length > 0 && ts.isStringLiteral(node.arguments[0])) {
                const modulePath = node.arguments[0].text;
                const resolvedPath = this.resolveModulePath(modulePath, fromFile, projectRoot, tsConfig);

                if (resolvedPath) {
                    return {
                        from: this.normalizePath(fromFile),
                        to: resolvedPath,
                        type: 'dynamic-import',
                        strength: 'MODERATE',
                        line: this.getLineNumber(node),
                        context: this.getNodeText(node).slice(0, 100)
                    };
                }
            }
        }

        // Handle require.resolve() calls
        if (ts.isPropertyAccessExpression(node.expression) &&
            ts.isIdentifier(node.expression.expression) &&
            node.expression.expression.text === 'require' &&
            ts.isIdentifier(node.expression.name) &&
            node.expression.name.text === 'resolve') {
            
            if (node.arguments.length > 0 && ts.isStringLiteral(node.arguments[0])) {
                const modulePath = node.arguments[0].text;
                const resolvedPath = this.resolveModulePath(modulePath, fromFile, projectRoot, tsConfig);

                if (resolvedPath) {
                    return {
                        from: this.normalizePath(fromFile),
                        to: resolvedPath,
                        type: 'require-resolve',
                        strength: 'WEAK',
                        line: this.getLineNumber(node),
                        context: this.getNodeText(node).slice(0, 100)
                    };
                }
            }
        }

        return null;
    }

    /**
     * Analyze import type expressions
     */
    analyzeImportType(node, fromFile, projectRoot, tsConfig) {
        if (node.argument && ts.isLiteralTypeNode(node.argument) && 
            ts.isStringLiteral(node.argument.literal)) {
            
            const modulePath = node.argument.literal.text;
            const resolvedPath = this.resolveModulePath(modulePath, fromFile, projectRoot, tsConfig);

            if (resolvedPath) {
                return {
                    from: this.normalizePath(fromFile),
                    to: resolvedPath,
                    type: 'import-type',
                    strength: 'MODERATE',
                    line: this.getLineNumber(node),
                    context: this.getNodeText(node).slice(0, 100)
                };
            }
        }

        return null;
    }

    /**
     * Check if import clause is type-only
     */
    isTypeOnlyImport(importClause) {
        // Check if all named imports are type-only
        if (importClause.namedBindings && ts.isNamedImports(importClause.namedBindings)) {
            return importClause.namedBindings.elements.every(element => element.isTypeOnly);
        }
        return false;
    }

    /**
     * Resolve module path to actual file path
     */
    resolveModulePath(modulePath, fromFile, projectRoot, tsConfig) {
        // Skip external modules (not relative paths)
        if (!modulePath.startsWith('.')) {
            return null;
        }

        const cacheKey = `${fromFile}:${modulePath}`;
        if (this.moduleResolutionCache.has(cacheKey)) {
            return this.moduleResolutionCache.get(cacheKey);
        }

        let resolvedPath = null;

        try {
            // Use TypeScript's module resolution if possible
            const result = ts.resolveModuleName(
                modulePath,
                fromFile,
                tsConfig.options,
                ts.sys
            );

            if (result.resolvedModule) {
                resolvedPath = this.normalizePath(
                    path.relative(projectRoot, result.resolvedModule.resolvedFileName)
                );
            }
        } catch (error) {
            // Fall back to manual resolution
            resolvedPath = this.manualModuleResolution(modulePath, fromFile, projectRoot);
        }

        // Verify the resolved file exists in our file map
        if (resolvedPath && !this.fileMap.has(resolvedPath)) {
            resolvedPath = null;
        }

        this.moduleResolutionCache.set(cacheKey, resolvedPath);
        return resolvedPath;
    }

    /**
     * Manual module resolution fallback
     */
    manualModuleResolution(modulePath, fromFile, projectRoot) {
        const fromDir = path.dirname(fromFile);
        const resolved = path.resolve(projectRoot, fromDir, modulePath);
        const relativePath = path.relative(projectRoot, resolved);
        const normalizedPath = this.normalizePath(relativePath);

        // Enhanced extension candidates for mixed JS/TS codebases
        const candidates = [
            normalizedPath,
            normalizedPath + '.ts',
            normalizedPath + '.tsx',
            normalizedPath + '.mts',
            normalizedPath + '.cts',
            normalizedPath + '.js',
            normalizedPath + '.jsx',
            normalizedPath + '.mjs',
            normalizedPath + '.cjs',
            normalizedPath + '.d.ts',
            normalizedPath + '.json',
            path.join(normalizedPath, 'index.ts'),
            path.join(normalizedPath, 'index.tsx'),
            path.join(normalizedPath, 'index.js'),
            path.join(normalizedPath, 'index.jsx'),
            path.join(normalizedPath, 'index.mjs'),
            path.join(normalizedPath, 'index.cjs'),
            path.join(normalizedPath, 'index.d.ts'),
            path.join(normalizedPath, 'package.json')
        ];

        for (const candidate of candidates) {
            if (this.fileMap.has(candidate)) {
                return candidate;
            }
        }

        return null;
    }

    /**
     * Analyze JSDoc comments for type dependencies
     */
    async analyzeJSDocDependencies(file, projectRoot, tsConfig) {
        const dependencies = [];
        
        try {
            const content = file.content || '';
            
            // RegExp to find JSDoc @type, @param, @returns with import() type expressions
            const jsDocImportRegex = /\/\*\*[\s\S]*?@(?:type|param|returns?)[\s\S]*?import\(['"`]([^'"`]+)['"`]\)/g;
            
            let match;
            while ((match = jsDocImportRegex.exec(content)) !== null) {
                const modulePath = match[1];
                const resolvedPath = this.resolveModulePath(modulePath, file.path, projectRoot, tsConfig);
                
                if (resolvedPath) {
                    dependencies.push({
                        from: this.normalizePath(file.path),
                        to: resolvedPath,
                        type: 'jsdoc-import',
                        strength: 'MODERATE',
                        line: this.getLineFromIndex(content, match.index),
                        context: match[0].slice(0, 100) + (match[0].length > 100 ? '...' : '')
                    });
                }
            }

            // Also look for @typedef with module references
            const typedefRegex = /\/\*\*[\s\S]*?@typedef[\s\S]*?{import\(['"`]([^'"`]+)['"`]\)/g;
            
            while ((match = typedefRegex.exec(content)) !== null) {
                const modulePath = match[1];
                const resolvedPath = this.resolveModulePath(modulePath, file.path, projectRoot, tsConfig);
                
                if (resolvedPath) {
                    dependencies.push({
                        from: this.normalizePath(file.path),
                        to: resolvedPath,
                        type: 'jsdoc-typedef',
                        strength: 'MODERATE',
                        line: this.getLineFromIndex(content, match.index),
                        context: match[0].slice(0, 100) + (match[0].length > 100 ? '...' : '')
                    });
                }
            }
            
        } catch (error) {
            this.errors.push(`Failed to analyze JSDoc in ${file.path}: ${error.message}`);
        }

        return dependencies;
    }

    /**
     * Helper methods
     */
    normalizePath(filePath) {
        return filePath.replace(/\\/g, '/');
    }

    getLineNumber(node) {
        if (node.getSourceFile) {
            const sourceFile = node.getSourceFile();
            const lineAndChar = sourceFile.getLineAndCharacterOfPosition(node.getStart());
            return lineAndChar.line + 1;
        }
        return 0;
    }

    getNodeText(node) {
        if (node.getSourceFile) {
            const sourceFile = node.getSourceFile();
            return sourceFile.text.substring(node.getStart(), node.getEnd()).trim();
        }
        return '';
    }

    getLineFromIndex(content, index) {
        return content.slice(0, index).split('\n').length;
    }

    deduplicateDependencies(dependencies) {
        const seen = new Map();
        const unique = [];

        for (const dep of dependencies) {
            const key = `${dep.from}:${dep.to}:${dep.type}`;
            if (!seen.has(key)) {
                seen.set(key, true);
                unique.push(dep);
            }
        }

        return unique;
    }

    createResult(dependencies, filesAnalyzed, errors) {
        const analysisTime = Date.now() - this.startTime;

        return {
            dependencies: dependencies,
            metadata: {
                filesAnalyzed: filesAnalyzed,
                analysisTime: `${analysisTime}ms`,
                pluginName: "typescript-analyzer",
                pluginVersion: "2.0.0"
            },
            errors: errors
        };
    }
}

/**
 * Main execution function
 */
async function main() {
    try {
        // Read input from stdin
        let inputData = '';
        process.stdin.setEncoding('utf8');

        process.stdin.on('readable', () => {
            let chunk;
            while ((chunk = process.stdin.read()) !== null) {
                inputData += chunk;
            }
        });

        process.stdin.on('end', async () => {
            try {
                const input = JSON.parse(inputData);
                const analyzer = new TypeScriptAnalyzer();
                const result = await analyzer.analyze(input);
                console.log(JSON.stringify(result, null, 2));
            } catch (error) {
                const errorResult = {
                    dependencies: [],
                    errors: [error.message],
                    metadata: {
                        filesAnalyzed: 0,
                        analysisTime: '0ms',
                        pluginName: 'typescript-analyzer',
                        pluginVersion: '2.0.0'
                    }
                };
                console.log(JSON.stringify(errorResult, null, 2));
                process.exit(1);
            }
        });

    } catch (error) {
        const errorResult = {
            dependencies: [],
            errors: [error.message],
            metadata: {
                filesAnalyzed: 0,
                analysisTime: '0ms',
                pluginName: 'typescript-analyzer',
                pluginVersion: '2.0.0'
            }
        };
        console.log(JSON.stringify(errorResult, null, 2));
        process.exit(1);
    }
}

// Export for testing
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { TypeScriptAnalyzer };
}

// Run if called directly
if (require.main === module) {
    main();
}