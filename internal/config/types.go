package config

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
	Vars         []SetEntry
	EnvFileTmpls []EnvFileEntry
	Tasks        map[string]Task
	TaskNames    []string
	TaskfileDir  string
	Modules      []ModuleEntry
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
)

type SetEntry struct {
	Key     string
	ValTmpl string
	Secret  bool
}

type IntoEntry struct {
	ParentKey string
	ValueTmpl string
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
	DirTmpl string // working directory template (run: and call: steps)

	// KindCall
	CallTarget  string
	CallVars    []SetEntry
	IntoEntries []IntoEntry
	Soft        bool
	Interactive *bool

	// KindFor
	ForTarget string
	ForList   []string
	ForMatrix []ForMatrixEntry
	ForSteps  []Step

	// KindGet
	GetEntries []GetEntry
}
