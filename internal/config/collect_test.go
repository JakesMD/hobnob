package config

import (
	"testing"
)

func TestCollectGetParams(t *testing.T) {
	tests := []struct {
		name      string
		steps     []Step
		cfg       *ConfigFile
		wantNames []string
	}{
		{
			name:      "given steps with no get, when collected, then returns empty (why: no params to surface)",
			steps:     []Step{{Kind: KindRun, Command: "echo hi"}},
			wantNames: nil,
		},
		{
			name: "given steps with get entries, when collected, then returns all var names (why: params are inputs to the task)",
			steps: []Step{{Kind: KindGet, GetEntries: []GetEntry{
				{VarName: "ENV"},
				{VarName: "PORT", DefaultTmpl: "8080"},
			}}},
			wantNames: []string{"ENV", "PORT"},
		},
		{
			name: "given for loop containing get, when collected, then includes nested entries (why: params inside loops are still task inputs)",
			steps: []Step{{Kind: KindFor, ForSteps: []Step{{Kind: KindGet, GetEntries: []GetEntry{
				{VarName: "ITEM_CONFIRM"},
			}}}}},
			wantNames: []string{"ITEM_CONFIRM"},
		},
		{
			name: "given mixed steps with get and for-containing-get, when collected, then returns all (why: full param surface)",
			steps: []Step{
				{Kind: KindSet, SetEntries: []SetEntry{{Key: "X", ValTmpl: "1"}}},
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "REGION"}}},
				{Kind: KindFor, ForSteps: []Step{{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ZONE"}}}}},
				{Kind: KindRun, Command: "echo done"},
			},
			wantNames: []string{"REGION", "ZONE"},
		},
		{
			name: "given call step referencing sub-task with get, when collected, then includes sub-task params (why: --list misleads when called sub-task params are hidden)",
			steps: []Step{
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "TOP"}}},
				{Kind: KindCall, CallTarget: "_helper"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_helper": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "HELPER_VAR"}}},
					}},
				},
			},
			wantNames: []string{"TOP", "HELPER_VAR"},
		},
		{
			name: "given call step with template target, when collected, then skips dynamic call (why: cannot statically resolve template dispatch)",
			steps: []Step{
				{Kind: KindCall, CallTarget: "{{.PROFILE}}_setup"},
			},
			wantNames: nil,
		},
		{
			name: "given mutually recursive call tasks, when collected, then does not infinite loop (why: cycle guard prevents stack overflow)",
			steps: []Step{
				{Kind: KindCall, CallTarget: "b"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"b": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "FROM_B"}}},
						{Kind: KindCall, CallTarget: "a"},
					}},
					"a": {Steps: []Step{
						{Kind: KindCall, CallTarget: "b"},
					}},
				},
			},
			wantNames: []string{"FROM_B"},
		},
		{
			name: "given set before get for same var, when collected, then omits already-set var (why: set satisfies the var so no prompt needed)",
			steps: []Step{
				{Kind: KindSet, SetEntries: []SetEntry{{Key: "ENV", ValTmpl: "production"}}},
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}, {VarName: "PORT"}}},
			},
			wantNames: []string{"PORT"},
		},
		{
			name: "given call with: passes var, when sub-task has get for that var, then omits it (why: with: satisfies the sub-task var so no prompt needed)",
			steps: []Step{
				{Kind: KindCall, CallTarget: "_sub", CallVars: []SetEntry{{Key: "ENV", ValTmpl: "staging"}}},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_sub": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"REGION"},
		},
		{
			name: "given run into: captures var, when later sub-task has get for that var, then omits it (why: run output satisfies the var so no prompt needed)",
			steps: []Step{
				{Kind: KindRun, Command: "git rev-parse HEAD", IntoEntries: []IntoEntry{{ParentKey: "SHA", ValueTmpl: "stdout | trim"}}},
				{Kind: KindCall, CallTarget: "_sub"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_sub": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "SHA"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"REGION"},
		},
		{
			name: "given get in parent and same var in sub-task get, when collected, then deduplicates (why: already surfaced once so no double prompt)",
			steps: []Step{
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}}},
				{Kind: KindCall, CallTarget: "_sub"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_sub": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"ENV", "REGION"},
		},
		{
			name: "given for loop default ITEM iterator, when loop body has get for ITEM, then omits it (why: iterator is injected by loop engine not user input)",
			steps: []Step{
				{Kind: KindFor, ForSteps: []Step{
					{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ITEM"}, {VarName: "CONFIRM"}}},
				}},
			},
			wantNames: []string{"CONFIRM"},
		},
		{
			name: "given for matrix loop, when loop body has get for matrix vars, then omits them (why: matrix vars are injected by loop engine not user input)",
			steps: []Step{
				{Kind: KindFor, ForMatrix: []ForMatrixEntry{{VarName: "OS"}, {VarName: "ARCH"}}, ForSteps: []Step{
					{Kind: KindGet, GetEntries: []GetEntry{{VarName: "OS"}, {VarName: "ARCH"}, {VarName: "CONFIRM"}}},
				}},
			},
			wantNames: []string{"CONFIRM"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test.steps and test.cfg are the arrangement)

			// Act
			got := CollectGetParams(test.steps, test.cfg)

			// Assert
			if len(got) != len(test.wantNames) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(test.wantNames), got)
			}
			for i, name := range test.wantNames {
				if got[i].VarName != name {
					t.Errorf("entry[%d].VarName = %q, want %q", i, got[i].VarName, name)
				}
			}
		})
	}
}
