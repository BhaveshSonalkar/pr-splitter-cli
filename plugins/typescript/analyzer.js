#!/usr/bin/env node

/**
 * TypeScript/JavaScript Dependency Analyzer Plugin
 * 
 * This plugin uses the TypeScript Compiler API to perform accurate AST analysis
 * and detect dependencies between files for intelligent PR partitioning.
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
            pluginVersion: "1.0.0"
        }
    }));
    process.exit(1);
}

// Read input from stdin
let inputData = '';
process.stdin.setEncoding('utf8');

process.stdin.on('readable', () => {
    let chunk;
    while ((chunk = process.stdin.read()) !== null) {
        inputData += chunk;
    }
});

process.stdin.on('end', () => {
    const startTime = Date.now();
    
    try {
        const input = JSON.parse(inputData);
        const result = analyzeTypeScriptDependencies(input, startTime);
        console.log(JSON.stringify(result, null, 2));
    } catch (error) {
        console.error(JSON.stringify({
            dependencies: [],
            errors: [error.message],
            metadata: {
                filesAnalyzed: 0,
                analysisTime: `${Date.now() - startTime}ms`,
                pluginName: "typescript-analyzer",
                pluginVersion: "1.0.0"
            }
        }));
        process.exit(1);
    }
});

/**
 * Main analysis function
 */
function analyzeTypeScriptDependencies(input, startTime) {
    const { changedFiles, projectFiles, projectRoot } = input;
    const allFiles = [...changedFiles, ...projectFiles];
    
    // Filter TypeScript/JavaScript files
    const tsFiles = allFiles.filter(file => 
        file.path.match(/\.(ts|tsx|js|jsx)$/i)
    );
    
    if (tsFiles.length === 0) {
        return {
            dependencies: [],
            metadata: {
                filesAnalyzed: 0,
                analysisTime: `${Date.now() - startTime}ms`,
                pluginName: "typescript-analyzer",
                pluginVersion: "1.0.0"
            },
            errors: []
        };
    }
    
    // Create TypeScript program for analysis
    const { program, sourceFiles } = createTypeScriptProgram(tsFiles, projectRoot);
    
    // Analyze dependencies
    const dependencies = [];
    const errors = [];
    
    try {
        // Analyze each changed file for dependencies
        for (const file of changedFiles) {
            if (!file.path.match(/\.(ts|tsx|js|jsx)$/i)) {
                continue;
            }
            
            const sourceFile = sourceFiles.get(file.path);
            if (sourceFile) {
                const fileDeps = analyzeSourceFile(sourceFile, program, allFiles);
                dependencies.push(...fileDeps);
            }
        }
        
        // Remove duplicates
        const uniqueDependencies = removeDuplicateDependencies(dependencies);
        
        return {
            dependencies: uniqueDependencies,
            metadata: {
                filesAnalyzed: tsFiles.length,
                analysisTime: `${Date.now() - startTime}ms`,
                pluginName: "typescript-analyzer",
                pluginVersion: "1.0.0"
            },
            errors: errors
        };
        
    } catch (error) {
        errors.push(`Analysis error: ${error.message}`);
        
        return {
            dependencies: dependencies,
            metadata: {
                filesAnalyzed: tsFiles.length,
                analysisTime: `${Date.now() - startTime}ms`,
                pluginName: "typescript-analyzer",
                pluginVersion: "1.0.0"
            },
            errors: errors
        };
    }
}

/**
 * Create TypeScript program for analysis
 */
function createTypeScriptProgram(files, projectRoot) {
    // Create in-memory file system for TypeScript
    const fileMap = new Map();
    const sourceFiles = new Map();
    
    // Add files to in-memory system
    for (const file of files) {
        const fullPath = path.resolve(projectRoot, file.path);
        fileMap.set(fullPath, file.content);
    }
    
    // TypeScript compiler options
    const compilerOptions = {
        target: ts.ScriptTarget.ES2020,
        module: ts.ModuleKind.ESNext,
        moduleResolution: ts.ModuleResolutionKind.NodeJs,
        allowJs: true,
        checkJs: false,
        jsx: ts.JsxEmit.React,
        esModuleInterop: true,
        allowSyntheticDefaultImports: true,
        skipLibCheck: true,
        noResolve: false
    };
    
    // Custom compiler host
    const compilerHost = {
        getSourceFile: (fileName, languageVersion) => {
            const content = fileMap.get(fileName);
            if (content !== undefined) {
                const sourceFile = ts.createSourceFile(fileName, content, languageVersion, true);
                sourceFiles.set(path.relative(projectRoot, fileName).replace(/\\/g, '/'), sourceFile);
                return sourceFile;
            }
            
            // Try to read from file system for external dependencies
            try {
                if (fs.existsSync(fileName)) {
                    const content = fs.readFileSync(fileName, 'utf8');
                    return ts.createSourceFile(fileName, content, languageVersion, true);
                }
            } catch (err) {
                // Ignore file system errors
            }
            
            return undefined;
        },
        
        writeFile: () => {}, // No-op
        getCurrentDirectory: () => projectRoot,
        getDirectories: () => [],
        fileExists: (fileName) => fileMap.has(fileName) || fs.existsSync(fileName),
        readFile: (fileName) => fileMap.get(fileName) || (fs.existsSync(fileName) ? fs.readFileSync(fileName, 'utf8') : undefined),
        getCanonicalFileName: (fileName) => fileName,
        useCaseSensitiveFileNames: () => true,
        getNewLine: () => '\n'
    };
    
    // Create program
    const program = ts.createProgram(
        Array.from(fileMap.keys()),
        compilerOptions,
        compilerHost
    );
    
    return { program, sourceFiles };
}

/**
 * Analyze a source file for dependencies
 */
function analyzeSourceFile(sourceFile, program, allFiles) {
    const dependencies = [];
    const fileName = sourceFile.fileName;
    const checker = program.getTypeChecker();
    
    // Get relative path for consistent reporting
    const relativePath = path.relative(process.cwd(), fileName).replace(/\\/g, '/');
    
    // Visit all nodes in the AST
    function visit(node) {
        // Import declarations: import ... from "module"
        if (ts.isImportDeclaration(node)) {
            const dependency = analyzeImportDeclaration(node, relativePath, allFiles);
            if (dependency) {
                dependencies.push(dependency);
            }
        }
        
        // Export declarations: export ... from "module"
        else if (ts.isExportDeclaration(node)) {
            const dependency = analyzeExportDeclaration(node, relativePath, allFiles);
            if (dependency) {
                dependencies.push(dependency);
            }
        }
        
        // Call expressions: require("module"), import("module")
        else if (ts.isCallExpression(node)) {
            const dependency = analyzeCallExpression(node, relativePath, allFiles, checker);
            if (dependency) {
                dependencies.push(dependency);
            }
        }
        
        // Property access expressions for deeper analysis
        else if (ts.isPropertyAccessExpression(node)) {
            const dependency = analyzePropertyAccess(node, relativePath, allFiles, checker);
            if (dependency) {
                dependencies.push(dependency);
            }
        }
        
        // Continue traversing
        ts.forEachChild(node, visit);
    }
    
    visit(sourceFile);
    return dependencies;
}

/**
 * Analyze import declarations
 */
function analyzeImportDeclaration(node, fromFile, allFiles) {
    if (!node.moduleSpecifier || !ts.isStringLiteral(node.moduleSpecifier)) {
        return null;
    }
    
    const modulePath = node.moduleSpecifier.text;
    const resolvedPath = resolveModulePath(modulePath, fromFile, allFiles);
    
    if (!resolvedPath) {
        return null;
    }
    
    // Determine import type and strength
    let importType = 'import';
    let strength = 'CRITICAL'; // Imports are critical dependencies
    
    // Check if it's a type-only import
    if (node.importClause && node.importClause.isTypeOnly) {
        importType = 'type-import';
        strength = 'MODERATE'; // Type imports are moderate dependencies
    }
    
    return {
        from: fromFile,
        to: resolvedPath,
        type: importType,
        strength: strength,
        line: getLineNumber(node),
        context: getNodeText(node)
    };
}

/**
 * Analyze export declarations
 */
function analyzeExportDeclaration(node, fromFile, allFiles) {
    if (!node.moduleSpecifier || !ts.isStringLiteral(node.moduleSpecifier)) {
        return null;
    }
    
    const modulePath = node.moduleSpecifier.text;
    const resolvedPath = resolveModulePath(modulePath, fromFile, allFiles);
    
    if (!resolvedPath) {
        return null;
    }
    
    return {
        from: fromFile,
        to: resolvedPath,
        type: 'export',
        strength: 'CRITICAL', // Re-exports are critical
        line: getLineNumber(node),
        context: getNodeText(node)
    };
}

/**
 * Analyze call expressions (require, dynamic import)
 */
function analyzeCallExpression(node, fromFile, allFiles, checker) {
    // require() calls
    if (ts.isIdentifier(node.expression) && node.expression.text === 'require') {
        if (node.arguments.length > 0 && ts.isStringLiteral(node.arguments[0])) {
            const modulePath = node.arguments[0].text;
            const resolvedPath = resolveModulePath(modulePath, fromFile, allFiles);
            
            if (resolvedPath) {
                return {
                    from: fromFile,
                    to: resolvedPath,
                    type: 'require',
                    strength: 'STRONG', // require() is strong dependency
                    line: getLineNumber(node),
                    context: getNodeText(node)
                };
            }
        }
    }
    
    // Dynamic import() calls
    if (node.expression.kind === ts.SyntaxKind.ImportKeyword) {
        if (node.arguments.length > 0 && ts.isStringLiteral(node.arguments[0])) {
            const modulePath = node.arguments[0].text;
            const resolvedPath = resolveModulePath(modulePath, fromFile, allFiles);
            
            if (resolvedPath) {
                return {
                    from: fromFile,
                    to: resolvedPath,
                    type: 'dynamic-import',
                    strength: 'MODERATE', // Dynamic imports are moderate
                    line: getLineNumber(node),
                    context: getNodeText(node)
                };
            }
        }
    }
    
    return null;
}

/**
 * Analyze property access for method calls and deeper dependencies
 */
function analyzePropertyAccess(node, fromFile, allFiles, checker) {
    // This could be expanded to detect more complex dependencies
    // For now, we'll keep it simple and return null
    // Future enhancement: detect method calls across modules
    return null;
}

/**
 * Resolve module path to actual file path
 */
function resolveModulePath(modulePath, fromFile, allFiles) {
    // Skip external modules (no relative path)
    if (!modulePath.startsWith('.')) {
        return null;
    }
    
    // Resolve relative path
    const fromDir = path.dirname(fromFile);
    let resolved = path.join(fromDir, modulePath);
    resolved = path.normalize(resolved).replace(/\\/g, '/');
    
    // Create a map of available files for quick lookup
    const fileMap = new Map();
    for (const file of allFiles) {
        fileMap.set(file.path, file);
        
        // Also map without extension for easier lookup
        const withoutExt = file.path.replace(/\.(ts|tsx|js|jsx)$/, '');
        fileMap.set(withoutExt, file);
    }
    
    // Try different extensions and variations
    const candidates = [
        resolved,
        resolved + '.ts',
        resolved + '.tsx',
        resolved + '.js',
        resolved + '.jsx',
        resolved + '/index.ts',
        resolved + '/index.tsx',
        resolved + '/index.js',
        resolved + '/index.jsx'
    ];
    
    for (const candidate of candidates) {
        if (fileMap.has(candidate)) {
            return candidate;
        }
    }
    
    return null;
}

/**
 * Get line number of a node
 */
function getLineNumber(node) {
    if (node.getSourceFile) {
        const sourceFile = node.getSourceFile();
        const lineAndChar = sourceFile.getLineAndCharacterOfPosition(node.getStart());
        return lineAndChar.line + 1;
    }
    return 0;
}

/**
 * Get text content of a node
 */
function getNodeText(node) {
    if (node.getSourceFile) {
        return node.getSourceFile().text.substring(node.getStart(), node.getEnd()).trim();
    }
    return '';
}

/**
 * Remove duplicate dependencies
 */
function removeDuplicateDependencies(dependencies) {
    const seen = new Set();
    const unique = [];
    
    for (const dep of dependencies) {
        const key = `${dep.from}:${dep.to}:${dep.type}`;
        if (!seen.has(key)) {
            seen.add(key);
            unique.push(dep);
        }
    }
    
    return unique;
}