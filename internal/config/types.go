package config

import "hobnob/internal/value"

type Task struct {
	Info        string
	Dir         string // task-level working directory template
	IfExpr      string
	Interactive *bool
	Steps       []Step
	Hidden      bool        // true = omit from --list
	Cfg         *ConfigFile // non-nil = use this cfg for sub-calls (module task)
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

	// ModuleLayer/ModuleLayerSecrets hold the default-tier vars this file
	// contributes to its own subtree when reached as a module — its env:
	// block plus its vars: block, computed once in resolveModuleFile relative
	// to the parent scope it was loaded with. Applied at runtime via
	// Scope.SetIfDefault (see runner.applyModuleLayer): a default for the
	// subtree, never an override of something the caller (or a higher scope
	// layer) already supplied — matching how a root file's own env:/vars:
	// blocks are themselves just low layers BuildScope applies.
	//
	// ModuleConstLayer/ModuleConstLayerSecrets hold this file's own const:
	// block instead — unconditionally overwritten into the subtree's scope
	// (Scope.Set), since a module's own const: is a hard fact about that
	// module, not a default: ordinary lexical shadowing, the nearest
	// declaration wins.
	//
	// All four are nil for a root ConfigFile (never applied at runtime; see
	// runner.executeTask, gated on Task.Cfg != nil).
	ModuleLayer             map[string]value.Value
	ModuleLayerSecrets      map[string]bool
	ModuleConstLayer        map[string]value.Value
	ModuleConstLayerSecrets map[string]bool
}

// EnvFileEntry is one env: block entry. SecretOverride is nil unless the
// entry explicitly sets secret: true/false; nil means "use the default,
// secret: false" (see config.LoadEnvFiles).
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
	KindUse
)

type SetEntry struct {
	Key     string
	ValTmpl string
	ValNode *JSONNode // non-nil for a map/list literal; ValTmpl unused when set
	Secret  bool
}

type IntoEntry struct {
	ParentKey string
	ValueTmpl string
	ValNode   *JSONNode // non-nil for a nested object/array literal; ValueTmpl unused when set
}

// JSONNodeKind discriminates the shape of a JSONNode.
type JSONNodeKind int

const (
	// JSONString is a deferred leaf: a template (set:/with:) or source
	// expression (into:) evaluated at runtime, not parse time.
	JSONString JSONNodeKind = iota
	// JSONLiteral is a non-string scalar (bool/int/float/null) decoded at
	// parse time and passed through unevaluated — a {{ }} action can only
	// occur inside a YAML string scalar, so these are never templates.
	JSONLiteral
	JSONObject
	JSONArray
)

// JSONNode is a parsed literal JSON shape (a set:/with: map or list literal,
// or a nested into: object/array) whose string leaves are unevaluated
// template/source-expr text. Evaluating one means walking the tree,
// evaluating each JSONString leaf, and json.Marshal-ing the assembled
// result exactly once — never re-serializing text into an already-built
// JSON string, which is what let unescaped quotes/backslashes in evaluated
// leaf values corrupt the surrounding JSON.
type JSONNode struct {
	Kind     JSONNodeKind
	Tmpl     string      // JSONString
	Literal  any         // JSONLiteral
	Fields   []JSONField // JSONObject
	Elements []JSONNode  // JSONArray
}

type JSONField struct {
	Key  string
	Node JSONNode
}

type GetEntry struct {
	VarName     string
	Info        string
	FromList    []string
	FromTmpl    string
	Multi       bool
	Check       string
	DefaultTmpl string
	Secret      bool
	Optional    bool
}

type ForMatrixEntry struct {
	VarName  string
	List     []string
	ListTmpl string
}

type Step struct {
	Kind   StepKind
	IfExpr string

	// KindSet
	SetEntries []SetEntry

	// KindRun
	Command string
	Argv    []string // run: list form; element templates, mutually exclusive with Command
	DirTmpl string   // working directory template (run:, call:, and use: steps)

	// KindCall, KindUse
	CallTarget  string // call: target; also use:'s target
	CallVars    []SetEntry
	IntoEntries []IntoEntry
	Soft        bool
	Interactive *bool

	// KindUse
	Rerun bool

	// KindFor
	ForTarget string
	ForList   []string
	ForMatrix []ForMatrixEntry
	ForSteps  []Step

	// KindGet
	GetEntries []GetEntry
}
