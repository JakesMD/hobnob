// Package testconventions is a golangci-lint module plugin that enforces
// hobnob's test shape: every test carries a given/when/then comment with a
// (why: …) clause, as its doc comment or in its body, and its body is split
// into // Arrange, // Act and // Assert blocks, in that order. // Arrange may
// be left out when nothing runs before // Act.
package testconventions

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("testconventions", New)
}

// New builds the plugin; it takes no settings.
func New(any) (register.LinterPlugin, error) {
	return plugin{}, nil
}

type plugin struct{}

func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{Analyzer}, nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

// Analyzer reports tests that break the conventions.
var Analyzer = &analysis.Analyzer{
	Name: "testconventions",
	Doc:  "checks tests for a given/when/then (why: …) comment and // Arrange, // Act, // Assert markers",
	Run:  run,
}

var (
	gwtPattern = regexp.MustCompile(`(?is)^given\b.*\bwhen\b.*\bthen\b`)
	whyPattern = regexp.MustCompile(`(?s)\(why:\s*\S.*\)`)
	markers    = []string{"Arrange", "Act", "Assert"}
)

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && isTest(fn) {
				checkTest(pass, file, fn)
			}
		}
	}
	return nil, nil
}

// isTest mirrors go test's rule: a top-level TestXxx(t *testing.T) where Xxx
// does not start with a lowercase letter. TestMain is excluded.
func isTest(fn *ast.FuncDecl) bool {
	name := fn.Name.Name
	if fn.Recv != nil || fn.Body == nil || !strings.HasPrefix(name, "Test") || name == "TestMain" {
		return false
	}
	if rest := name[len("Test"):]; rest != "" && rest[0] >= 'a' && rest[0] <= 'z' {
		return false
	}
	params := fn.Type.Params.List
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
}

func checkTest(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl) {
	var groups []*ast.CommentGroup
	if fn.Doc != nil {
		groups = append(groups, fn.Doc)
	}
	for _, group := range file.Comments {
		if group.Pos() > fn.Body.Lbrace && group.End() < fn.Body.Rbrace {
			groups = append(groups, group)
		}
	}

	checkGWT(pass, fn, groups)
	checkMarkers(pass, fn, groups)
}

func checkGWT(pass *analysis.Pass, fn *ast.FuncDecl, groups []*ast.CommentGroup) {
	for _, group := range groups {
		text := strings.TrimSpace(group.Text())
		if !gwtPattern.MatchString(text) {
			continue
		}
		if !whyPattern.MatchString(text) {
			pass.Reportf(group.Pos(), "%s: given/when/then comment has no (why: …) clause", fn.Name.Name)
		}
		return
	}
	pass.Reportf(fn.Name.Pos(), "%s: no given/when/then comment", fn.Name.Name)
}

func checkMarkers(pass *analysis.Pass, fn *ast.FuncDecl, groups []*ast.CommentGroup) {
	first := map[string]token.Pos{}
	for _, group := range groups {
		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			for _, marker := range markers {
				if text == marker && first[marker] == token.NoPos {
					first[marker] = comment.Pos()
				}
			}
		}
	}

	var missing []string
	for _, marker := range markers {
		if first[marker] == token.NoPos && (marker != "Arrange" || codeBefore(fn, first["Act"])) {
			missing = append(missing, "// "+marker)
		}
	}
	if len(missing) > 0 {
		pass.Reportf(fn.Name.Pos(), "%s: missing %s", fn.Name.Name, strings.Join(missing, ", "))
		return
	}
	arrange := first["Arrange"]
	if (arrange != token.NoPos && arrange > first["Act"]) || first["Act"] > first["Assert"] {
		pass.Reportf(fn.Name.Pos(), "%s: // Arrange, // Act, // Assert must come in that order", fn.Name.Name)
	}
}

// codeBefore reports whether any statement finishes before pos, which is
// when a test has something to arrange. With no // Act at all, the answer is
// moot: the missing // Act is reported anyway.
func codeBefore(fn *ast.FuncDecl, pos token.Pos) bool {
	if pos == token.NoPos {
		return true
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.BlockStmt, *ast.EmptyStmt:
		case ast.Stmt:
			if stmt.End() < pos {
				found = true
			}
		}
		return !found
	})
	return found
}
