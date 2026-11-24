// Package glob is the matcher behind a get: entry's match:. It knows nothing
// of prompts or the picker: a Matcher answers "does this text, or this path,
// pass the patterns?", and the picker can hand Path to its listing as a
// predicate.
//
// A value passes if it matches any plain pattern (or there are none) and no
// "!" pattern. Matching is case-insensitive; * and ? stop at "/", ** crosses
// it, [abc] and {a,b} work (doublestar syntax).
package glob

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Matcher is a compiled set of patterns.
type Matcher struct {
	plain []string
	neg   []string
}

// New compiles patterns, failing on a malformed one.
func New(patterns []string) (Matcher, error) {
	var matcher Matcher
	for _, raw := range patterns {
		pat := strings.ToLower(raw)
		negated := strings.HasPrefix(pat, "!")
		pat = strings.TrimPrefix(pat, "!")
		if !doublestar.ValidatePattern(pat) {
			return Matcher{}, fmt.Errorf("bad pattern %q", raw)
		}
		if negated {
			matcher.neg = append(matcher.neg, pat)
		} else {
			matcher.plain = append(matcher.plain, pat)
		}
	}
	return matcher, nil
}

// Empty reports whether there are no patterns at all.
func (m Matcher) Empty() bool { return len(m.plain)+len(m.neg) == 0 }

// Text matches the whole of text, for menu items and text answers.
func (m Matcher) Text(text string) bool {
	text = strings.ToLower(text)
	return m.pass(func(pat string) bool { return matches(pat, text) })
}

// Path matches an absolute path, for picked files and directories. A pattern
// with no "/" is matched against the base name; one with a "/" inside, the
// end of the path at any depth; a leading "/" or "~/" the absolute path
// (home standing for "~"); a leading "./" the path from workDir.
func (m Matcher) Path(abs, workDir, home string) bool {
	return m.Included(abs, workDir, home) && !m.Excluded(abs, workDir, home)
}

// Included reports whether the plain patterns accept abs: true when there
// are none.
func (m Matcher) Included(abs, workDir, home string) bool {
	if len(m.plain) == 0 {
		return true
	}
	for _, pat := range m.plain {
		if pathMatches(pat, abs, workDir, home) {
			return true
		}
	}
	return false
}

// Excluded reports whether any "!" pattern matches abs.
func (m Matcher) Excluded(abs, workDir, home string) bool {
	for _, pat := range m.neg {
		if pathMatches(pat, abs, workDir, home) {
			return true
		}
	}
	return false
}

func (m Matcher) pass(match func(string) bool) bool {
	for _, pat := range m.neg {
		if match(pat) {
			return false
		}
	}
	if len(m.plain) == 0 {
		return true
	}
	for _, pat := range m.plain {
		if match(pat) {
			return true
		}
	}
	return false
}

func matches(pat, text string) bool {
	ok, err := doublestar.Match(pat, text)
	return err == nil && ok
}

func pathMatches(pat, abs, workDir, home string) bool {
	abs = strings.ToLower(filepath.ToSlash(abs))
	switch {
	case strings.HasPrefix(pat, "~/"):
		return underMatches(pat[2:], abs, home)
	case strings.HasPrefix(pat, "./"):
		return underMatches(pat[2:], abs, workDir)
	case strings.HasPrefix(pat, "/"):
		return matches(pat, abs)
	case strings.Contains(pat, "/"):
		return matches("**/"+pat, abs)
	default:
		return matches(pat, abs[strings.LastIndex(abs, "/")+1:])
	}
}

// underMatches matches pat against abs taken relative to root.
func underMatches(pat, abs, root string) bool {
	root = strings.ToLower(filepath.ToSlash(filepath.Clean(root)))
	rel, ok := strings.CutPrefix(abs, strings.TrimSuffix(root, "/")+"/")
	return ok && matches(pat, rel)
}
