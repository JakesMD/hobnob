package eval

import (
	"fmt"
	"strings"

	"hobnob/internal/value"
)

// IntoSource resolves the head name of an into: leaf to its value: stdout,
// stderr or exit for run:, a child var for call:. An absent name comes back
// as a value.Missing sentinel, so a later "| default" can catch it; a name
// the source can never hold is a real error.
type IntoSource func(name string) (value.Value, error)

// EvalIntoLeaf evaluates one into: leaf. A leaf containing {{ }} is a
// template over vars — the caller's own evolving scope, so a later entry can
// build on an earlier one. Any other leaf is a head name, with an optional
// leading ".", looked up in source, followed by whatever accessor and filter
// chain the leaf goes on with ("stdout[0].name | trim", "NAME|upper"). That
// tail is evaluated as template syntax, not split on a literal " | ", with
// vars in scope so a dynamic key (stdout[.KEY]) reads the caller's scope.
func EvalIntoLeaf(leaf string, source IntoSource, vars map[string]value.Value) (value.Value, error) {
	if strings.Contains(leaf, "{{") {
		return EvalValue(leaf, vars)
	}
	leaf = strings.TrimPrefix(strings.TrimSpace(leaf), ".")
	head, tail := splitIntoHead(leaf)
	if head == "" {
		return value.Value{}, fmt.Errorf("into: %q does not start with a name", leaf)
	}
	src, err := source(head)
	if err != nil {
		return value.Value{}, err
	}
	if strings.TrimSpace(tail) == "" {
		if src.IsMissing() {
			return value.Value{}, src.MissingErr()
		}
		return src, nil
	}
	return evalChainOn(src, tail, vars)
}

// splitIntoHead splits an into: leaf into its leading name and everything
// after it: "stdout[0].name | trim" -> ("stdout", "[0].name | trim");
// "NAME|trim" -> ("NAME", "|trim").
func splitIntoHead(leaf string) (head, tail string) {
	if leaf == "" || !isIdentStart(leaf[0]) {
		return "", leaf
	}
	i := 1
	for i < len(leaf) && isIdentCont(leaf[i]) {
		i++
	}
	return leaf[:i], leaf[i:]
}
