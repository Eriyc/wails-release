package frontend

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

func TestPhase5FrontendContractExportsExist(t *testing.T) {
	decls := frontendExportedDecls(t)

	for _, name := range []string{"LayeredFS", "BundleArtifact", "CheckCompat"} {
		if _, ok := decls[name]; !ok {
			t.Errorf("expected pkg/frontend to export %s for phase 5", name)
		}
	}
}

func frontendExportedDecls(t *testing.T) map[string]struct{} {
	t.Helper()

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse frontend package: %v", err)
	}

	decls := make(map[string]struct{})
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch typed := decl.(type) {
				case *ast.FuncDecl:
					decls[typed.Name.Name] = struct{}{}
				case *ast.GenDecl:
					for _, spec := range typed.Specs {
						switch spec := spec.(type) {
						case *ast.TypeSpec:
							decls[spec.Name.Name] = struct{}{}
						case *ast.ValueSpec:
							for _, name := range spec.Names {
								decls[name.Name] = struct{}{}
							}
						}
					}
				}
			}
		}
	}
	return decls
}
