package indexer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

func ParseGo(path string, maxFileSize int64) ([]Symbol, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlinks not supported: %s", path)
	}

	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("file too large to parse: %d bytes (limit: %d)", info.Size(), maxFileSize)
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	var symbols []Symbol

	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := Symbol{
				Name: d.Name.Name,
				Kind: "function",
				Line: fset.Position(d.Pos()).Line,
			}
			if d.Recv != nil {
				sym.Kind = "method"
			}
			if d.Doc != nil {
				sym.Doc = d.Doc.Text()
			}
			symbols = append(symbols, sym)
		case *ast.GenDecl:
			var kind string
			switch d.Tok {
			case token.TYPE:
				kind = "type"
			case token.CONST:
				kind = "const"
			case token.VAR:
				kind = "var"
			default:
				continue
			}

			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && kind == "type" {
					sym := Symbol{
						Name: ts.Name.Name,
						Kind: kind,
						Line: fset.Position(ts.Pos()).Line,
					}
					if ts.Doc != nil {
						sym.Doc = ts.Doc.Text()
					}
					symbols = append(symbols, sym)
				} else if vs, ok := spec.(*ast.ValueSpec); ok && (kind == "var" || kind == "const") {
					for _, name := range vs.Names {
						sym := Symbol{
							Name: name.Name,
							Kind: kind,
							Line: fset.Position(name.Pos()).Line,
						}
						if vs.Doc != nil {
							sym.Doc = vs.Doc.Text()
						} else if d.Doc != nil {
							sym.Doc = d.Doc.Text()
						}
						sym.Doc = strings.TrimSpace(sym.Doc)
						symbols = append(symbols, sym)
					}
				}
			}
		}
	}
	return symbols, nil
}
