package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

type analyzerPlugin struct{}

func (*analyzerPlugin) GetAnalyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		{
			Name: "imports",
			Doc:  "Custom import analysis",
			Run:  analyzeImports,
		},
		{
			Name: "tests",
			Doc:  "Custom test analysis",
			Run:  analyzeTests,
		},
	}
}

var AnalyzerPlugin analyzerPlugin

type restrictedImport struct {
	Regex      *regexp.Regexp
	Exceptions []any // string prefix, regex, or func(pkg *types.Package, from *analysis.Pass) bool
}

func matchTestPackages(_ *types.Package, from *analysis.Pass) bool {
	return strings.HasSuffix(from.Pkg.Name(), "_test")
}

func analyzeImports(p *analysis.Pass) (any, error) {
	restrictions := []restrictedImport{
		{
			Regex: regexp.MustCompile("^github.com/rancher/opni/plugins/"),
			Exceptions: []any{
				"github.com/rancher/opni/plugins/",
				matchTestPackages,
				"github.com/rancher/opni/cmd/",
				"github.com/rancher/opni/pkg/opni/",
				"github.com/rancher/opni/internal/cmd/",
				"github.com/rancher/opni/pkg/agent/v1",
			},
		},
		{
			Regex: regexp.MustCompile("^github.com/rancher/opni/pkg/test"),
			Exceptions: []any{
				"github.com/rancher/opni/pkg/test",
				"github.com/rancher/opni/test/",
				regexp.MustCompile(`^github.com/rancher/opni/plugins/\w+/test$`),
				matchTestPackages,
				"github.com/rancher/opni/internal/mage/",
				"github.com/rancher/opni/internal/cmd/testenv",
			},
		},
		{
			Regex: regexp.MustCompile("^github.com/rancher/opni/test"),
		},
		{
			Regex: regexp.MustCompile("^github.com/rancher/opni/web"),
			Exceptions: []any{
				"github.com/rancher/opni/pkg/opni/commands",
				"github.com/rancher/opni/pkg/dashboard",
			},
		},
		{
			Regex: regexp.MustCompile("^github.com/rancher/opni/pkg/dashboard"),
			Exceptions: []any{
				"github.com/rancher/opni/pkg/opni/commands",
				"github.com/rancher/opni/internal/cmd/testenv",
			},
		},
		{
			Regex: regexp.MustCompile("^github.com/rancher/opni/controllers"),
			Exceptions: []any{
				"github.com/rancher/opni/controllers",
				"github.com/rancher/opni/pkg/opni/commands",
			},
		},
		{
			Regex: regexp.MustCompile("^github.com/rancher/opni/pkg/opni/commands"),
			Exceptions: []any{
				"github.com/rancher/opni/pkg/opni",
			},
		},
	}

	pkgPath := p.Pkg.Path()

	skip := map[string]struct{}{}
	var visit func(pkg *types.Package, trace []string)
	visit = func(pkg *types.Package, trace []string) {
		path := pkg.Path()
		if !strings.HasPrefix(path, "github.com/rancher/opni") {
			skip[path] = struct{}{}
			return
		}
		if _, ok := skip[path]; ok {
			return
		}

		trace = append(trace, path)
		for _, imp := range pkg.Imports() {
			visit(imp, trace)
		}

		var matchingRestriction *restrictedImport
		for _, restriction := range restrictions {
			if restriction.Regex.MatchString(path) {
				matchingRestriction = &restriction
				break
			}
		}
		if matchingRestriction == nil {
			return
		}
		// check exceptions
		var exempt bool
		for _, exception := range matchingRestriction.Exceptions {
			switch e := exception.(type) {
			case string:
				exempt = strings.HasPrefix(pkgPath, e)
			case *regexp.Regexp:
				exempt = e.MatchString(pkgPath)
			case func(*types.Package, *analysis.Pass) bool:
				exempt = e(pkg, p)
			default:
				panic("bug: unknown exception type")
			}
			if exempt {
				skip[path] = struct{}{}
				return
			}
		}

		var importTrace string
		for i, t := range trace {
			importTrace += strings.Repeat("  ", i) + t + "\n"
		}

		// find which file is importing the restricted import
		for _, f := range p.Files {
			for _, i := range f.Imports {
				if i.Path.Value == "\""+path+"\"" {
					msg := fmt.Sprintf("importing %s from %s is not allowed\nFull import trace:\n%s\nStarting at:", path, pkgPath, importTrace)
					p.Report(analysis.Diagnostic{
						Pos:      i.Pos(),
						Category: "imports",
						Message:  msg,
					})
				}
			}
		}
	}
	visit(p.Pkg, nil)
	return nil, nil
}

func analyzeTests(p *analysis.Pass) (any, error) {
	// - Test files must have a package name ending in _test
	// - All ginkgo suites must have labels
	// - All ginkgo suites must import "github.com/rancher/opni/pkg/test/setup"
	var hasTests, hasSuite, hasGinkgoTests bool
	allLabels := map[string]struct{}{}
	for _, f := range p.Files {
		name := p.Fset.File(f.Pos()).Name()
		hasTestFilenameSuffix := strings.HasSuffix(name, "_test.go")
		hasTestPackageSuffix := strings.HasSuffix(p.Pkg.Name(), "_test")
		hasTestImports := len(findTestImports(f)) > 0
		if hasTestFilenameSuffix || hasTestPackageSuffix {
			// search for any test functions
			if !hasTests == false {
				hasTests = hasGoTests(f)
			}
			if !hasSuite && strings.HasSuffix(name, "suite_test.go") {
				hasSuite = true

				// check for setup import
				var hasSetupImport bool
				for _, imp := range f.Imports {
					if imp.Path.Value == `"github.com/rancher/opni/pkg/test/setup"` {
						hasSetupImport = true
						break
					}
				}
				if !hasSetupImport {
					p.Report(analysis.Diagnostic{
						Pos:      f.Pos(),
						Category: "ginkgo",
						Message:  "test suite is missing import `_ \"github.com/rancher/opni/pkg/test/setup\"`",
					})
				}
			}
			if isGinkgo(f) {
				hasGinkgoTests = true
				labels, found := findLabels(f)
				if found && len(labels) == 0 {
					p.Report(analysis.Diagnostic{
						Pos:      f.Name.Pos(),
						End:      f.Name.End(),
						Category: "ginkgo",
						Message:  "test is missing labels",
					})
				}
				for _, label := range labels {
					allLabels[label] = struct{}{}
				}
			}
		}
		if hasTestImports && !(hasTestFilenameSuffix || hasTestPackageSuffix) {
			p.Report(analysis.Diagnostic{
				Pos:      f.Name.Pos(),
				End:      f.Name.End(),
				Category: "imports",
				Message:  "tests must have a package name ending in _test or a file name ending in _test.go",
			})
		}
	}
	if hasGinkgoTests && hasTests && !hasSuite {
		p.Report(analysis.Diagnostic{
			Pos:      p.Files[0].Name.Pos(),
			Message:  fmt.Sprintf("package %s does not contain a ginkgo suite", p.Pkg.Name()),
			Category: "ginkgo",
		})
	}
	if _, ok := allLabels["integration"]; ok {
		for _, f := range p.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				fn, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := fn.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				if ident.Name == "BeforeSuite" {
					// within a BeforeSuite, check for the presence of test.Environment
					// and ensure that it is wrapped by a call to testruntime.IfIntegration
					// example:
					// var _ = BeforeSuite(func() {
					//   testruntime.IfIntegration(func() {
					//     env := test.Environment{}
					//     ...
					var hasTestEnv token.Pos
					var hasIfIntegration token.Pos
					ast.Inspect(fn, func(n ast.Node) bool {
						switch n := n.(type) {
						case *ast.SelectorExpr:
							if n.Sel.Name == "IfIntegration" {
								hasIfIntegration = n.Pos()
								return false
							}
						case *ast.CompositeLit:
							ident, ok := n.Type.(*ast.SelectorExpr)
							if ok {
								if ident.Sel.Name == "Environment" {
									hasTestEnv = n.Pos()
									return false
								}
							}
						}
						return true
					})

					if (hasTestEnv != token.NoPos) && (hasIfIntegration == token.NoPos) {
						p.Report(analysis.Diagnostic{
							Pos:      hasTestEnv,
							End:      hasTestEnv,
							Category: "ginkgo",
							Message:  "usage of test.Environment in BeforeSuite must be guarded by testruntime.IfIntegration",
						})
					}
					return false
				}
				return true
			})
		}
	}
	return nil, nil
}

func hasGoTests(file *ast.File) (hasTests bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if !strings.HasPrefix(funcDecl.Name.Name, "Test") {
			return false
		}
		if funcDecl.Type.Params.NumFields() != 1 {
			return false
		}
		for _, param := range funcDecl.Type.Params.List {
			if starExpr, ok := param.Type.(*ast.StarExpr); ok {
				selectorExpr, ok := starExpr.X.(*ast.SelectorExpr)
				if ok && selectorExpr.Sel.Name == "T" {
					hasTests = true
					return false
				}
			}
		}
		return true
	})
	return
}

func findTestEnvironment(node ast.Node) (hasTestEnv bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		compositeLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		ident, ok := compositeLit.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if ident.Sel.Name == "Environment" {
			hasTestEnv = true
			return false
		}

		return true
	})
	return
}

var topLevelNodeNames = map[string]struct{}{
	"Describe":  {},
	"FDescribe": {},
	"XDescribe": {},
}

func isGinkgo(file *ast.File) bool {
	for _, i := range file.Imports {
		if i.Path.Value == "\"github.com/onsi/ginkgo/v2\"" {
			return true
		}
	}
	return false
}

func findLabels(file *ast.File) ([]string, bool) {
	var labels []string
	var found bool

	ast.Inspect(file, func(node ast.Node) bool {
		callExpr, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := callExpr.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		if _, ok := topLevelNodeNames[ident.Name]; ok {
			found = true
			for _, arg := range callExpr.Args {
				call, ok := arg.(*ast.CallExpr)
				if !ok {
					continue
				}

				ident, ok := call.Fun.(*ast.Ident)
				if !ok {
					continue
				}

				if ident.Name == "Label" {
					for _, labelArg := range call.Args {
						if lit, ok := labelArg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							labels = append(labels, strings.Trim(lit.Value, `"`))
						}
					}
				}
			}
		}

		return false
	})

	return labels, found
}

func findTestImports(file *ast.File) []*ast.ImportSpec {
	var imports []*ast.ImportSpec
	for _, i := range file.Imports {
		name := strings.Trim(i.Name.String(), `"`)
		if name == "testing" ||
			strings.HasPrefix(name, "github.com/onsi/ginkgo") ||
			strings.HasPrefix(name, "github.com/onsi/gomega") ||
			strings.HasPrefix(name, "github.com/onsi/biloba") ||
			strings.HasPrefix(name, "github.com/rancher/opni/pkg/test") ||
			strings.HasPrefix(name, "github.com/rancher/opni/test") {
			imports = append(imports, i)
		}
	}
	return imports
}
