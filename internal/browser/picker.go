package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hobnob/internal/ask"
	"hobnob/internal/value"
)

// KindPath is the prompt kind of a file or folder picker.
const KindPath = "path"

// Picker is a picker prompt's own part of the snapshot: the Finder-style
// column view down to the open folder.
type Picker struct {
	Type    string   `json:"type"` // "file" or "dir"
	Open    string   `json:"open"`
	Columns []Column `json:"columns"`
	// Selected is what the picker starts with picked; the page tracks the
	// picks after that.
	Selected []string `json:"selected"`
}

// Column lists one folder, already filtered. Next names the entry that the
// following column is the listing of; it is empty in the last column.
type Column struct {
	Path    string  `json:"path"`
	Next    string  `json:"next,omitempty"`
	Entries []Entry `json:"entries"`
}

// Entry is a file or folder that is listed. A folder that isn't Pickable can
// still be opened.
type Entry struct {
	Name     string `json:"name"`
	Dir      bool   `json:"dir,omitempty"`
	Pickable bool   `json:"pickable,omitempty"`
}

// pickerView lists a picker's folders, by the rules its question carries.
type pickerView struct {
	rules ask.PathRules
	home  string
}

func newPickerView(rules ask.PathRules) pickerView {
	home, _ := os.UserHomeDir()
	return pickerView{rules: rules, home: home}
}

func (r pickerView) list(dir string) []Entry {
	listed := r.rules.List(dir)
	entries := make([]Entry, len(listed))
	for i, e := range listed {
		entries[i] = Entry{Name: e.Name, Dir: e.Dir, Pickable: e.Pickable}
	}
	return entries
}

// view lists the columns from the home folder, or the filesystem root when
// open isn't under it, down to open.
func (r pickerView) view(open string, selected []string) *Picker {
	root := string(filepath.Separator)
	if r.home != "" && (open == r.home || strings.HasPrefix(open, r.home+string(filepath.Separator))) {
		root = r.home
	}
	chain := []string{open}
	for dir := open; dir != root; {
		dir = filepath.Dir(dir)
		chain = append([]string{dir}, chain...)
	}
	view := &Picker{Type: pickerType(r.rules), Open: open, Selected: selected, Columns: []Column{}}
	for i, dir := range chain {
		column := Column{Path: dir, Entries: r.list(dir)}
		if i+1 < len(chain) {
			column.Next = filepath.Base(chain[i+1])
		}
		view.Columns = append(view.Columns, column)
	}
	if view.Selected == nil {
		view.Selected = []string{}
	}
	return view
}

func pickerType(rules ask.PathRules) string {
	if rules.Dir {
		return "dir"
	}
	return "file"
}

// start is the folder the page opens and what it starts with picked. A
// folder picker opens the folder its default names, as picking one opens it
// on the page, even when match: keeps it from starting picked.
func (r pickerView) start(multi bool) (open string, selected []string) {
	open, selected = r.rules.Opening(multi)
	if target := r.rules.DefaultTarget(); r.rules.Dir && target != "" {
		open = target
	}
	return open, selected
}

// readPaths reads a picker's answer: the picked paths, each absolute as the
// page lists them.
func readPaths(multi bool) func(json.RawMessage) (ask.Answer, string) {
	return func(raw json.RawMessage) (ask.Answer, string) {
		var paths []string
		if multi {
			if err := json.Unmarshal(raw, &paths); err != nil {
				return ask.Answer{}, "the answer must be a list of paths"
			}
		} else {
			var one string
			if err := json.Unmarshal(raw, &one); err != nil {
				return ask.Answer{}, "the answer must be a path"
			}
			if one != "" {
				paths = []string{one}
			}
		}
		for _, path := range paths {
			if !filepath.IsAbs(path) {
				return ask.Answer{}, fmt.Sprintf("%s isn't an absolute path", path)
			}
		}
		return ask.Answer{Picks: paths}, ""
	}
}

// promptPath asks for a file or folder in the page. The page only reads what
// the person picked into an ask.Answer; q's judge rules on it, as it does
// for the terminal.
func (s *state) promptPath(ctx context.Context, q ask.Question) (value.Value, error) {
	view := newPickerView(*q.Path)
	open, selected := view.start(q.Multi)
	p := Prompt{Kind: KindPath, VarName: q.VarName, Info: q.Info, Optional: q.Optional,
		Multi: q.Multi, Picker: view.view(open, selected)}
	read := readPaths(q.Multi)
	var answer value.Value
	skipped, err := s.askPicker(ctx, p, view, func(raw json.RawMessage) (RailEntry, string, bool) {
		given, refusal := read(raw)
		if refusal != "" {
			return RailEntry{}, refusal, false
		}
		verdict := q.Judge(given)
		if verdict.Refusal != "" {
			return RailEntry{}, verdict.Refusal, false
		}
		if verdict.Skipped {
			return RailEntry{}, "", true
		}
		answer = verdict.Value
		entry := RailEntry{Label: label(q.Info, q.VarName), Value: shown(answer)}
		if q.Secret {
			entry.Value = maskedAnswer
		}
		return entry, "", false
	})
	if err != nil {
		return value.Value{}, err
	}
	if skipped {
		return q.Judge(ask.Answer{}).Value, nil
	}
	return answer, nil
}

// askPicker is ask for a picker, which can also change its open folder.
func (s *state) askPicker(ctx context.Context, p Prompt, view pickerView, judge judge) (bool, error) {
	selected := p.Picker.Selected
	return s.ask(ctx, p, judge, func(open *openPrompt) {
		open.browse = func(path string) error {
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() || !filepath.IsAbs(path) {
				return fmt.Errorf("%s isn't a folder to open", path)
			}
			listing := view.view(filepath.Clean(path), selected)
			return s.settle(open, func() { open.Picker = listing })
		}
	})
}

type openRequest struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

// openFolder is the POST /actions/open handler: a picker's open folder
// changes, and the next snapshot lists it.
func (s *state) openFolder(body json.RawMessage) error {
	var req openRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	open, err := s.openPrompt(req.ID)
	if err != nil {
		return err
	}
	if open.browse == nil {
		return fmt.Errorf("%s has no folder to open", open.VarName)
	}
	return open.browse(req.Path)
}
