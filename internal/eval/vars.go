package eval

import "strings"

// CloneMap returns a shallow copy of src — always non-nil, even for a nil or
// empty src, so callers can write into the result unconditionally.
func CloneMap[K comparable, V any](src map[K]V) map[K]V {
	dst := make(map[K]V, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

// SplitKV splits raw on its first "=" into a key/value pair. ok is false when
// raw has no "=" or an empty key (idx<=0) — the shared parsing rule for
// KEY=VALUE lines across CLI args, os.Environ(), env files, and env dumps.
func SplitKV(raw string) (key, val string, ok bool) {
	idx := strings.IndexByte(raw, '=')
	if idx <= 0 {
		return "", "", false
	}
	return raw[:idx], raw[idx+1:], true
}
