package ask

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PathRules are the rules of a type: file|dir entry, in one place: how a
// written path resolves, what makes a path pickable, where a picker opens and
// what it starts with picked. A picker (the terminal's, the page's) presents
// them; Question.Judge and Verify enforce them.
//
// Included and Excluded are the caller's predicates for match:, which ask
// never learns about:
//   - Included is the plain patterns: in a file picker it hides non-matching
//     files (never directories); in a dir picker non-matching directories can
//     be opened but not picked.
//   - Excluded is the "!" patterns: whatever it reports true for is hidden,
//     directories included.
//
// Either may be nil.
type PathRules struct {
	// Dir makes it a directory picker, else a file picker.
	Dir bool
	// Invoke is the absolute directory a picked or command-line path resolves
	// against.
	Invoke string
	// WorkDir is the absolute directory of the asking task: relative defaults
	// resolve against it and the picker opens in it. Empty means Invoke.
	WorkDir string
	// Defaults are as written in the taskfile.
	Defaults []string
	Included func(abs string, isDir bool) bool
	Excluded func(abs string, isDir bool) bool
	// Match is the match: patterns as text, for a refusal.
	Match string
	// VarName, when set, makes a kind refusal name the variable ("X is a
	// directory; CONF needs a file"), as a preset value's refusal does; a
	// picker's refusal stays short.
	VarName string
	// Home stands for ~; empty means the user's home directory.
	Home string
}

// PathEntry is a file or folder a picker lists. A folder that isn't Pickable
// can still be opened.
type PathEntry struct {
	Name     string
	Dir      bool
	Pickable bool
}

// junkNames are never listed or picked, on every OS. Compared
// case-insensitively; "._*" is handled as a prefix.
var junkNames = []string{
	".DS_Store", ".Spotlight-V100", ".Trashes", ".fseventsd", ".TemporaryItems",
	"Icon\r", "Thumbs.db", "desktop.ini", "$RECYCLE.BIN", "System Volume Information",
}

// IsJunk reports whether name is never listed or picked.
func IsJunk(name string) bool {
	if strings.HasPrefix(name, "._") {
		return true
	}
	for _, junk := range junkNames {
		if strings.EqualFold(name, junk) {
			return true
		}
	}
	return false
}

func (r PathRules) workDir() string {
	if r.WorkDir == "" {
		return r.Invoke
	}
	return r.WorkDir
}

// Resolve makes written absolute: a leading ~ is the home directory, a
// relative path hangs off base (empty written is base itself), and the
// result is cleaned. Symlinks are left alone.
func (r PathRules) Resolve(written, base string) (string, error) {
	switch {
	case written == "":
		return filepath.Clean(base), nil
	case written == "~" || strings.HasPrefix(written, "~/") || strings.HasPrefix(written, "~"+string(filepath.Separator)):
		home := r.Home
		if home == "" {
			var err error
			if home, err = os.UserHomeDir(); err != nil {
				return "", fmt.Errorf("can't expand ~ in %s: %w", written, err)
			}
		}
		return filepath.Join(home, written[1:]), nil
	case filepath.IsAbs(written):
		return filepath.Clean(written), nil
	}
	return filepath.Join(base, written), nil
}

// Allows reports whether abs could be picked: not junk, not Excluded, and
// Included.
func (r PathRules) Allows(abs string, isDir bool) bool {
	return !IsJunk(filepath.Base(abs)) &&
		(r.Excluded == nil || !r.Excluded(abs, isDir)) &&
		(r.Included == nil || r.Included(abs, isDir))
}

func (r PathRules) wrongKind(is, needs string) string {
	if r.VarName == "" {
		return fmt.Sprintf(" is %s, not a %s", is, needs)
	}
	return fmt.Sprintf(" is %s; %s needs a %s", is, r.VarName, needs)
}

// Verify resolves written against base and checks it exists, is the right
// kind and is allowed. refusal says why not, naming the path.
func (r PathRules) Verify(written, base string) (abs, refusal string) {
	abs, err := r.Resolve(written, base)
	if err != nil {
		return "", err.Error()
	}
	info, err := os.Stat(abs) // follows a symlink: its kind is its target's
	switch {
	case os.IsNotExist(err):
		return "", abs + " doesn't exist"
	case err != nil:
		return "", fmt.Sprintf("can't read %s: %v", abs, err)
	case r.Dir && !info.IsDir():
		return "", abs + r.wrongKind("a file", "directory")
	case !r.Dir && info.IsDir():
		return "", abs + r.wrongKind("a directory", "file")
	case IsJunk(filepath.Base(abs)):
		return "", abs + " is a system file name"
	case !r.Allows(abs, info.IsDir()):
		return "", fmt.Sprintf("%s doesn't match %s", abs, r.Match)
	}
	return abs, ""
}

// Opening is the folder the picker lists first and the paths it starts
// picked. The first default decides the folder: a directory given to a file
// picker is the folder itself, any other default opens in its folder, and a
// missing one in the nearest folder that exists; with no default it is the
// work directory. The defaults of the picker's kind that are allowed start
// picked, only the first unless multi.
func (r PathRules) Opening(multi bool) (open string, picks []string) {
	open, _ = r.Resolve("", r.workDir())
	for i, written := range r.Defaults {
		abs, err := r.Resolve(written, r.workDir())
		if err != nil {
			continue
		}
		info, statErr := os.Stat(abs)
		if i == 0 {
			open = filepath.Dir(abs)
			if statErr == nil && info.IsDir() && !r.Dir {
				open = abs
			}
		}
		if (i == 0 || multi) && statErr == nil && info.IsDir() == r.Dir && r.Allows(abs, info.IsDir()) {
			picks = append(picks, abs)
		}
	}
	return NearestFolder(open), picks
}

// DefaultTarget is the first default when it exists and is of the picker's
// kind, allowed or not: the entry a picker points at, or "" when there is
// none.
func (r PathRules) DefaultTarget() string {
	if len(r.Defaults) == 0 {
		return ""
	}
	abs, err := r.Resolve(r.Defaults[0], r.workDir())
	if err != nil {
		return ""
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() != r.Dir {
		return ""
	}
	return abs
}

// NearestFolder is dir, or its nearest ancestor that exists as a folder.
func NearestFolder(dir string) string {
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			return dir
		}
		dir = next
	}
}

// List lists dir for a picker, applying the junk list, the kind and the
// predicates. Directories come first, then by name ignoring case.
func (r PathRules) List(dir string) []PathEntry {
	items, _ := os.ReadDir(dir)
	var dirs, files []PathEntry
	for _, item := range items {
		if IsJunk(item.Name()) {
			continue
		}
		abs := filepath.Join(dir, item.Name())
		isDir := item.IsDir()
		if item.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(abs); err == nil {
				isDir = info.IsDir()
			}
		}
		if r.Dir && !isDir {
			continue
		}
		if r.Excluded != nil && r.Excluded(abs, isDir) {
			continue
		}
		included := r.Included == nil || r.Included(abs, isDir)
		switch {
		case isDir:
			dirs = append(dirs, PathEntry{item.Name(), true, r.Dir && included})
		case included:
			files = append(files, PathEntry{item.Name(), false, true})
		}
	}
	byName := func(s []PathEntry) {
		sort.SliceStable(s, func(i, j int) bool { return strings.ToLower(s[i].Name) < strings.ToLower(s[j].Name) })
	}
	byName(dirs)
	byName(files)
	return append(dirs, files...)
}
