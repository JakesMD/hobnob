// Package literal is the reference and literal grammar of every templated
// slot. A field's raw YAML parses here once, at load: a reference (.NAME,
// with accessors and filters) drops its braces, a list or map literal
// becomes a tree of template leaves, and a spread (REF...) is accepted only
// as a list element. What comes back evaluates against the scope at run
// time, so nothing is rendered before execution reaches it.
package literal

import (
	"gopkg.in/yaml.v3"

	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// Expr is what a templated single-value or literal slot holds: one template,
// or a list/map literal tree whose string leaves are templates. The zero Expr
// is an absent field and evaluates as "" does.
type Expr struct {
	tmpl string
	tree *node
}

// Single parses a field that holds one value. A bare reference is wrapped in
// {{ }} so it keeps its type; any other text is stored as written. A spread
// is an error wrapping ErrSpread: there is no list for its items to join.
func Single(raw string) (Expr, error) {
	tmpl, err := singleLeaf(templateLeaf, raw)
	if err != nil {
		return Expr{}, err
	}
	return Expr{tmpl: tmpl}, nil
}

// Parse parses the value of a set:, with:, const: or vars: entry named name.
// A scalar is Single; a mapping or sequence is a literal whose string leaves
// are templates, whose other scalars are decoded as typed values, and whose
// list elements may spread. name roots the field path in error messages
// (B.k, B[0]).
func Parse(name string, n *yaml.Node) (Expr, error) {
	if n.Kind != yaml.MappingNode && n.Kind != yaml.SequenceNode {
		return Single(n.Value)
	}
	tree, err := parseTree(n, templateLeaf, name)
	if err != nil {
		return Expr{}, err
	}
	return Expr{tree: &tree}, nil
}

// Value evaluates e keeping its type: a lone reference resolves to the
// variable's value, a literal to the structure it builds, and any other
// template to the String it renders.
func (e Expr) Value(vars map[string]value.Value) (value.Value, error) {
	if e.tree == nil {
		return eval.EvalValue(e.tmpl, vars)
	}
	tree, err := e.tree.eval(func(leaf string) (value.Value, error) {
		return eval.EvalValue(leaf, vars)
	})
	if err != nil {
		return value.Value{}, err
	}
	return value.Of(tree), nil
}

// Text renders e as text, the way a command line or a message reads it.
func (e Expr) Text(vars map[string]value.Value) (string, error) {
	if e.tree == nil {
		return eval.EvalTemplate(e.tmpl, vars)
	}
	val, err := e.Value(vars)
	if err != nil {
		return "", err
	}
	return val.String(), nil
}

// Refs returns every variable name e reads, walking a literal's leaves; a
// spread counts as a read of its reference.
func (e Expr) Refs() ([]string, error) {
	if e.tree == nil {
		return eval.ReferencedVars(e.tmpl)
	}
	return e.tree.refs()
}

// IsZero reports whether e is the zero Expr: the field was absent or "".
func (e Expr) IsZero() bool {
	return e.tree == nil && e.tmpl == ""
}

// String returns the stored template text of a single value (a reference
// with its braces restored), or "" for a literal.
func (e Expr) String() string {
	return e.tmpl
}
