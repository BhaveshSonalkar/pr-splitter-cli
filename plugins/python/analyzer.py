#!/usr/bin/env python3

"""
Python Dependency Analyzer Plugin

This plugin uses Python's AST module to analyze import statements
and detect dependencies between Python files for intelligent PR partitioning.

Input: JSON via stdin with changed files and project context
Output: JSON to stdout with dependency relationships
"""

import json
import sys
import ast
import os
import time
from pathlib import Path
from typing import List, Dict, Any, Optional

class PythonDependencyAnalyzer:
    def __init__(self):
        self.start_time = time.time()
        
    def analyze(self, input_data: Dict[str, Any]) -> Dict[str, Any]:
        """Main analysis function"""
        changed_files = input_data.get('changedFiles', [])
        project_files = input_data.get('projectFiles', [])
        project_root = input_data.get('projectRoot', os.getcwd())
        
        all_files = changed_files + project_files
        
        # Filter Python files
        python_files = [f for f in all_files if self._is_python_file(f['path'])]
        
        if not python_files:
            return self._create_result([], 0, [])
        
        # Create file mapping for quick lookup
        file_map = {f['path']: f for f in all_files}
        
        # Analyze dependencies
        dependencies = []
        errors = []
        
        try:
            for file_info in changed_files:
                if not self._is_python_file(file_info['path']):
                    continue
                    
                file_deps = self._analyze_file(file_info, file_map, project_root)
                dependencies.extend(file_deps)
                
        except Exception as e:
            errors.append(f"Analysis error: {str(e)}")
        
        # Remove duplicates
        unique_deps = self._remove_duplicates(dependencies)
        
        return self._create_result(unique_deps, len(python_files), errors)
    
    def _is_python_file(self, file_path: str) -> bool:
        """Check if file is a Python file"""
        return file_path.endswith(('.py', '.pyi'))
    
    def _analyze_file(self, file_info: Dict[str, Any], file_map: Dict[str, Any], project_root: str) -> List[Dict[str, Any]]:
        """Analyze a single Python file for dependencies"""
        dependencies = []
        
        try:
            # Parse the Python file using AST
            tree = ast.parse(file_info['content'], filename=file_info['path'])
            
            # Visit all nodes to find import statements
            for node in ast.walk(tree):
                dep = None
                
                if isinstance(node, ast.Import):
                    # Handle: import module, import module.submodule
                    dep = self._analyze_import(node, file_info['path'], file_map, project_root)
                    
                elif isinstance(node, ast.ImportFrom):
                    # Handle: from module import something
                    dep = self._analyze_import_from(node, file_info['path'], file_map, project_root)
                
                if dep:
                    dependencies.append(dep)
                    
        except SyntaxError as e:
            # Skip files with syntax errors (might be Python 2, etc.)
            pass
        except Exception as e:
            # Log other errors but continue
            pass
            
        return dependencies
    
    def _analyze_import(self, node: ast.Import, from_file: str, file_map: Dict[str, Any], project_root: str) -> Optional[Dict[str, Any]]:
        """Analyze import statements"""
        for alias in node.names:
            module_path = self._resolve_module_path(alias.name, from_file, file_map, project_root)
            if module_path:
                return {
                    'from': from_file,
                    'to': module_path,
                    'type': 'import',
                    'strength': 'CRITICAL',
                    'line': node.lineno,
                    'context': f"import {alias.name}"
                }
        return None
    
    def _analyze_import_from(self, node: ast.ImportFrom, from_file: str, file_map: Dict[str, Any], project_root: str) -> Optional[Dict[str, Any]]:
        """Analyze from...import statements"""
        if not node.module:
            # Relative import without module (from . import something)
            return None
            
        module_path = self._resolve_module_path(node.module, from_file, file_map, project_root, node.level)
        if module_path:
            imported_items = [alias.name for alias in node.names]
            return {
                'from': from_file,
                'to': module_path,
                'type': 'from-import',
                'strength': 'STRONG',
                'line': node.lineno,
                'context': f"from {node.module} import {', '.join(imported_items)}"
            }
        return None
    
    def _resolve_module_path(self, module_name: str, from_file: str, file_map: Dict[str, Any], project_root: str, level: int = 0) -> Optional[str]:
        """Resolve module name to actual file path"""
        # Skip standard library and external modules
        if self._is_standard_library(module_name):
            return None
            
        # Handle relative imports
        if level > 0:
            # Relative import
            from_dir = Path(from_file).parent
            for _ in range(level - 1):
                from_dir = from_dir.parent
            base_path = from_dir / module_name.replace('.', '/')
        else:
            # Absolute import
            base_path = Path(project_root) / module_name.replace('.', '/')
        
        # Try different file extensions and __init__.py
        candidates = [
            str(base_path) + '.py',
            str(base_path / '__init__.py'),
            str(base_path) + '.pyi'
        ]
        
        for candidate in candidates:
            # Convert to relative path from project root
            try:
                rel_path = os.path.relpath(candidate, project_root).replace('\\', '/')
                if rel_path in file_map:
                    return rel_path
            except ValueError:
                # Path is not relative to project root
                continue
                
        return None
    
    def _is_standard_library(self, module_name: str) -> bool:
        """Check if module is part of Python standard library"""
        stdlib_modules = {
            'os', 'sys', 'json', 'time', 'datetime', 'pathlib', 'ast', 're',
            'collections', 'itertools', 'functools', 'operator', 'typing',
            'math', 'random', 'string', 'io', 'logging', 'unittest', 'urllib',
            'http', 'email', 'xml', 'html', 'csv', 'sqlite3', 'threading',
            'multiprocessing', 'subprocess', 'socket', 'ssl', 'hashlib',
            'base64', 'pickle', 'copy', 'tempfile', 'shutil', 'glob'
        }
        
        # Check if it's a known stdlib module or starts with stdlib prefix
        root_module = module_name.split('.')[0]
        return root_module in stdlib_modules
    
    def _remove_duplicates(self, dependencies: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Remove duplicate dependencies"""
        seen = set()
        unique = []
        
        for dep in dependencies:
            key = f"{dep['from']}:{dep['to']}:{dep['type']}"
            if key not in seen:
                seen.add(key)
                unique.append(dep)
                
        return unique
    
    def _create_result(self, dependencies: List[Dict[str, Any]], files_analyzed: int, errors: List[str]) -> Dict[str, Any]:
        """Create the result JSON"""
        analysis_time = int((time.time() - self.start_time) * 1000)
        
        return {
            'dependencies': dependencies,
            'metadata': {
                'filesAnalyzed': files_analyzed,
                'analysisTime': f"{analysis_time}ms",
                'pluginName': 'python-analyzer',
                'pluginVersion': '1.0.0'
            },
            'errors': errors
        }

def main():
    """Main entry point"""
    try:
        # Read input from stdin
        input_data = json.load(sys.stdin)
        
        # Create analyzer and run analysis
        analyzer = PythonDependencyAnalyzer()
        result = analyzer.analyze(input_data)
        
        # Output result as JSON
        json.dump(result, sys.stdout, indent=2)
        
    except Exception as e:
        error_result = {
            'dependencies': [],
            'errors': [str(e)],
            'metadata': {
                'filesAnalyzed': 0,
                'analysisTime': '0ms',
                'pluginName': 'python-analyzer',
                'pluginVersion': '1.0.0'
            }
        }
        json.dump(error_result, sys.stdout, indent=2)
        sys.exit(1)

if __name__ == '__main__':
    main() 