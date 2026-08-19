package eval

import "hobnob/internal/value"

// sv wraps a plain string map as typed scope vars — a test convenience
// shared across eval package tests, where the string-vs-Value distinction is
// incidental to what's under test (template rendering, condition
// evaluation), not the type system itself.
func sv(m map[string]string) map[string]value.Value {
	out := make(map[string]value.Value, len(m))
	for k, v := range m {
		out[k] = value.Str(v)
	}
	return out
}
