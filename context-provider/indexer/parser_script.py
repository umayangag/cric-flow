import ast
import json
import sys

class SymbolVisitor(ast.NodeVisitor):
    def __init__(self):
        self.symbols = []
        self._class_stack = []

    def visit_ClassDef(self, node):
        self.symbols.append({
            "name": node.name,
            "kind": "class",
            "line": node.lineno,
            "doc": ast.get_docstring(node) or ""
        })
        self._class_stack.append(node.name)
        self.generic_visit(node)
        self._class_stack.pop()

    def visit_FunctionDef(self, node):
        self._handle_func(node)

    def visit_AsyncFunctionDef(self, node):
        self._handle_func(node)

    def _handle_func(self, node):
        kind = "method" if self._class_stack else "function"
        self.symbols.append({
            "name": node.name,
            "kind": kind,
            "line": node.lineno,
            "doc": ast.get_docstring(node) or ""
        })
        self.generic_visit(node)

def parse(path):
    try:
        with open(path, "r", encoding="utf-8", errors="ignore") as f:
            source = f.read()
        
        tree = ast.parse(source, filename=path)
        visitor = SymbolVisitor()
        visitor.visit(tree)
        print(json.dumps(visitor.symbols, ensure_ascii=False))
    except Exception as e:
        sys.stderr.write(str(e))
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit(1)
    parse(sys.argv[1])
