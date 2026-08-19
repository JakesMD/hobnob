package eval

import "path/filepath"

// ResolvePath returns path unchanged if absolute, else joins it onto base —
// the shared "resolve a config-relative path" rule used for run:/call: dir:,
// env: file paths, and module file paths, all of which are relative to the
// hobnob file unless given as an absolute path.
func ResolvePath(path, base string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
