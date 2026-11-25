package web

// Screens a Snapshot can show.
const (
	ScreenTasks    = "tasks"
	ScreenPrompt   = "prompt"
	ScreenProgress = "progress"
	ScreenDone     = "done"
	ScreenFailed   = "failed"
)

// Outcomes of a finished run.
const (
	OutcomeDone      = "done"
	OutcomeFailed    = "failed"
	OutcomeCancelled = "cancelled"
)

// States of a rail entry.
const (
	StateRunning = "running"
	StateDone    = "done"
	StateFailed  = "failed"
)

// Snapshot is the whole state the page shows. Every SSE event carries one,
// so a page that reconnects needs no replay.
type Snapshot struct {
	Screen   string    `json:"screen"`
	Tasks    []Task    `json:"tasks,omitempty"`
	Task     Task      `json:"task"`
	Rail     Rail      `json:"rail"`
	Prompt   *Prompt   `json:"prompt,omitempty"`
	Progress *Progress `json:"progress,omitempty"`
	Result   *Result   `json:"result,omitempty"`
}

// Task is a task: the outermost task of the run, or one the task picker offers.
type Task struct {
	Name string `json:"name"`
	Info string `json:"info"`
}

// Rail is the left-hand list: what has been answered, and the setup so far.
type Rail struct {
	Answers []RailEntry `json:"answers"`
	Setup   []RailEntry `json:"setup"`
}

// RailEntry is one line of the rail.
type RailEntry struct {
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
	State string `json:"state,omitempty"`
}

// Progress is the progress screen: the innermost running task's label and the
// last line of output, empty for a held step.
type Progress struct {
	Label     string `json:"label"`
	LastLine  string `json:"lastLine"`
	ElapsedMS int64  `json:"elapsedMs"`
	// Cancelling is true once Cancel was pressed and the run is stopping.
	Cancelling bool `json:"cancelling"`
}

// Result is the Done, Failed or Cancelled screen.
type Result struct {
	Outcome    string `json:"outcome"`
	Label      string `json:"label"`
	LastLine   string `json:"lastLine"`
	DurationMS int64  `json:"durationMs"`
	Log        string `json:"log"`
	Home       bool   `json:"home"`
}
