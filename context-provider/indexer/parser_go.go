package indexer

import (
	"go/ast"
	"go/parser"
	"go/token"
)

func ParseGo(path string) []Symbol {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil
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
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						sym := Symbol{
							Name: ts.Name.Name,
							Kind: "type",
							Line: fset.Position(ts.Pos()).Line,
						}
						if d.Doc != nil {
							sym.Doc = d.Doc.Text()
						}
						symbols = append(symbols, sym)
					}
				}
			}
		}
	}
	return symbols
}
