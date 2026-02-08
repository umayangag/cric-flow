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

def get_symbols(path):
    with open(path, "r", encoding="utf-8", errors="ignore") as f:
        source = f.read()
    
    tree = ast.parse(source, filename=path)
    visitor = SymbolVisitor()
    visitor.visit(tree)
    return visitor.symbols

if __name__ == "__main__":
    if len(sys.argv) > 1:
        try:
            syms = get_symbols(sys.argv[1])
            print(json.dumps(syms, ensure_ascii=False))
        except Exception as e:
            sys.stderr.write(str(e))
            sys.exit(1)
    else:
        # Loop mode
        # Ensure stdout uses utf-8
        if hasattr(sys.stdout, 'reconfigure'):
            sys.stdout.reconfigure(encoding='utf-8')
            
        for line in sys.stdin:
            path = line.strip()
            if not path: continue
            try:
                syms = get_symbols(path)
                print(json.dumps(syms, ensure_ascii=False), flush=True)
            except Exception as e:
                # Send error to Go
                err_msg = str(e)
                print(json.dumps({"error": err_msg}, ensure_ascii=False), flush=True)
