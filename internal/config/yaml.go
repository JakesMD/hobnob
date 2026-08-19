package config

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// bareVarRef matches a bare .VAR reference with an optional pipe chain,
// e.g. ".RELEASE_LIST" or ".RELEASE_LIST | first". Relative paths (./infra,
// ../tests) and dotfiles (.git) never match — they contain "/" or a
// lowercase-leading segment, which this pattern excludes.
var bareVarRef = regexp.MustCompile(`^\.[A-Z][A-Z0-9_]*(\s*\|[^{]+)?$`)

// normalizeTmpl wraps a bare .VAR or .VAR | filter expression in {{ }} so
// users can write `default: .LIST | first` instead of `default: '{{.LIST | first}}'`.
func normalizeTmpl(value string) string {
	if bareVarRef.MatchString(value) {
		return "{{" + value + "}}"
	}
	return value
}

// isEmptyNode reports whether node is a YAML null (a key given with no value,
// e.g. "vars:" followed by nothing, or explicit "vars: ~"/"vars: null").
// Top-level vars:/modules:/tasks: blocks tolerate this as "no entries".
func isEmptyNode(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

// validateVarName rejects a var name containing template syntax — names are
// used as map keys, not rendered, so a "{{" in one is always a mistake.
func validateVarName(name string) error {
	if strings.Contains(name, "{{") {
		return fmt.Errorf("variable name %q must not contain template syntax", name)
	}
	return nil
}

// parseBool reports whether n's scalar value is the literal string "true" —
// hobnob's YAML bool fields (interactive, secret, multi, soft, optional)
// only ever hold unquoted true/false, so any other value (including a
// template) is simply false rather than an error.
func parseBool(node *yaml.Node) bool {
	return node.Value == "true"
}

// mapEntry is one key/value pair of a yaml.Node mapping, as returned by
// mapEntries.
type mapEntry struct {
	Key string
	Val *yaml.Node
}

// mapEntries returns a yaml.Node mapping's Content as key/value pairs, so
// callers can range over entries instead of each hand-rolling the stride-2
// walk that a yaml.Node mapping stores its pairs as.
func mapEntries(content []*yaml.Node) []mapEntry {
	entries := make([]mapEntry, 0, len(content)/2)
	for i := 0; i+1 < len(content); i += 2 {
		entries = append(entries, mapEntry{Key: content[i].Value, Val: content[i+1]})
	}
	return entries
}
