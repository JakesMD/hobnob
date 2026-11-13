package config

import "hobnob/internal/literal"

type Task struct {
	Info   string
	Dir    literal.Expr // task-level working directory
	IfExpr string
	Once   bool // memoize this task's result per hobnob invocation (see runner.execCall)
	// Results names the vars this task promises exist after any call to it;
	// a call: with no into: pulls exactly these (see runner.execCall).
	Results []string
	Steps   []Step
	Hidden  bool        // true = omit from --list
	Cfg     *ConfigFile // non-nil = use this cfg for sub-calls (module task)
}

type ModuleEntry struct {
	Prefix      string
	FileTmpl    string
	ShowTmpls   []string
	HideTmpls   []string
	FlattenTmpl string
}

type ConfigFile struct {
	FilePath     string
	EnvFileTmpls []EnvFileEntry
	ConstEntries []SetEntry // const: — outranks env, env files, and CLI args
	VarEntries   []SetEntry // vars: — overridable; below CLI args, above env files
	Tasks        map[string]Task
	TaskNames    []string
	TaskfileDir  string
	Modules      []ModuleEntry
}

// EnvFileEntry is one env: block entry. SecretOverride is nil unless the
// entry explicitly sets secret: true/false; nil means "use the default,
// secret: false" (see scope.loadEnvFiles).
type EnvFileEntry struct {
	PathTmpl       string
	SecretOverride *bool
}

type StepKind int

const (
	KindRun StepKind = iota
	KindSet
	KindCall
	KindFor
	KindGet
)

type SetEntry struct {
	Key    string
	Val    literal.Expr // a single value or a map/list literal
	Secret bool
}

type IntoEntry struct {
	ParentKey string
	Val       literal.Into
	Bare      bool // written as a bare NAME, short for NAME: .NAME (call: only)
}

// BareIntoEntry is the entry a bare into: NAME parses to, and what a call:
// without into: pulls for each of its target's results: NAME: NAME.
func BareIntoEntry(name string) IntoEntry {
	return IntoEntry{ParentKey: name, Val: literal.IntoLeaf(name), Bare: true}
}

// DeclaresConst reports whether name is one of this file's own const: names.
func (cfg *ConfigFile) DeclaresConst(name string) bool {
	for _, entry := range cfg.ConstEntries {
		if entry.Key == name {
			return true
		}
	}
	return false
}

type GetEntry struct {
	VarName     string
	Info        string
	FromList    []string
	FromTmpl    literal.Expr
	Multi       bool
	Check       string
	DefaultTmpl literal.Expr
	Secret      bool
	Optional    bool
}

type ForMatrixEntry struct {
	VarName  string
	List     []string
	ListTmpl literal.Expr
}

// Step is one step of any kind. Which fields a kind may fill, and which
// names it writes, are in its row of stepKinds (stepkinds.go).
type Step struct {
	Kind   StepKind
	IfExpr string

	// KindSet
	SetEntries []SetEntry

	// KindRun
	Command  literal.Expr
	Argv     literal.Argv // run: list form, mutually exclusive with Command
	DirTmpl  literal.Expr // working directory (run: and call:)
	Soft     bool         // continue past a non-zero exit instead of halting the timeline (run: and call:)
	Quiet    bool         // suppress run: output on success; replayed in full on failure
	QuietMsg literal.Expr // shown in place of suppressed output; empty is valid (run: only)

	// KindCall
	CallTarget  string // call: target
	CallVars    []SetEntry
	IntoEntries []IntoEntry
	HasInto     bool // into: was written, even as []; a call: without it takes the target's results:

	// KindFor
	ForTarget literal.Expr
	ForList   []string
	ForMatrix []ForMatrixEntry
	ForSteps  []Step

	// KindGet
	GetEntries []GetEntry
}
