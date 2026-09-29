package scope_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hobnob/internal/config"
	"hobnob/internal/scope"
	"hobnob/internal/value"
)

// writeFiles writes each fixture into a fresh temp dir and returns the path
// of its hobnob.yml.
func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return filepath.Join(dir, "hobnob.yml")
}

func TestLoadAndApply(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		cliVars map[string]string
		osEnv   map[string]string
		// task, when set, names a module task whose file scope is applied to
		// a copy of the root scope — what the runner does when it starts one.
		task string
		// runtimeSet is written into that copy before Apply, standing in for
		// a caller's set:/with:.
		runtimeSet  map[string]string
		want        map[string]string
		wantAbsent  []string
		wantSecrets []string
		wantErr     string
	}{
		{
			name: "given a name set by every root layer, when loaded, then const: wins (why: const: outranks even a CLI arg)",
			files: map[string]string{
				"hobnob.yml": "const:\n  - X: const\nvars:\n  - X: vars\nenv:\n  - .env\ntasks: {}\n",
				".env":       "X=envfile\n",
			},
			cliVars: map[string]string{"X": "cli"},
			osEnv:   map[string]string{"X": "os"},
			want:    map[string]string{"X": "const"},
		},
		{
			name: "given a CLI arg, an env file and vars: for one name, when loaded, then the CLI arg wins (why: env < vars: < env files < CLI args)",
			files: map[string]string{
				"hobnob.yml": "vars:\n  - X: vars\nenv:\n  - .env\ntasks: {}\n",
				".env":       "X=envfile\n",
			},
			cliVars: map[string]string{"X": "cli"},
			want:    map[string]string{"X": "cli"},
		},
		{
			name: "given an env file and vars: for one name, when loaded, then the env file wins over vars: and vars: over the OS env",
			files: map[string]string{
				"hobnob.yml": "vars:\n  - X: vars\n  - Y: vars\nenv:\n  - .env\ntasks: {}\n",
				".env":       "X=envfile\n",
			},
			osEnv: map[string]string{"Y": "os"},
			want:  map[string]string{"X": "envfile", "Y": "vars"},
		},
		{
			name: "given a vars: entry built from an env file, when loaded, then it reads the env file's value (why: upward reads, ADR-0001)",
			files: map[string]string{
				"hobnob.yml": "vars:\n  - URL: \"https://{{.HOST}}/v1\"\nenv:\n  - .env\ntasks: {}\n",
				".env":       "HOST=staging.example.com\n",
			},
			want: map[string]string{"URL": "https://staging.example.com/v1"},
		},
		{
			name: "given a vars: entry whose template would fail and a CLI arg of the same name, when loaded, then it's skipped without being evaluated",
			files: map[string]string{
				"hobnob.yml": "vars:\n  - X: .NOPE.deeper\ntasks: {}\n",
			},
			cliVars: map[string]string{"X": "cli"},
			want:    map[string]string{"X": "cli"},
		},
		{
			name: "given a module env file and a CLI arg of the same name, when a module task starts, then the module's vars: builds from the CLI arg (why: a module's chain reads its importer's resolved scope)",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - m: ./mod.yml\ntasks: {}\n",
				"mod.yml":    "env:\n  - .env\nvars:\n  - LABEL: \"built-from-{{.STAGE}}\"\ntasks:\n  show:\n    steps: []\n",
				".env":       "STAGE=dev\n",
			},
			cliVars: map[string]string{"STAGE": "prod"},
			task:    "m:show",
			want:    map[string]string{"STAGE": "prod", "LABEL": "built-from-prod"},
		},
		{
			name: "given a module env file and a name only on the OS env, when a module task starts, then the module's env file fills it (why: an ambient name is a gap)",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - m: ./mod.yml\ntasks: {}\n",
				"mod.yml":    "env:\n  - .env\ntasks:\n  show:\n    steps: []\n",
				".env":       "STAGE=dev\n",
			},
			osEnv: map[string]string{"STAGE": "os"},
			task:  "m:show",
			want:  map[string]string{"STAGE": "dev"},
		},
		{
			name: "given a module const: and a CLI arg of the same name, when a module task starts, then the module const: wins (why: the nearest declaration wins)",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - m: ./mod.yml\ntasks: {}\n",
				"mod.yml":    "const:\n  - REGION: eu\ntasks:\n  show:\n    steps: []\n",
			},
			cliVars: map[string]string{"REGION": "us"},
			task:    "m:show",
			want:    map[string]string{"REGION": "eu"},
		},
		{
			name: "given a module vars: entry and a caller's own value for it, when a module task starts, then the caller's value stays (why: module vars: only fill a gap)",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - m: ./mod.yml\ntasks: {}\n",
				"mod.yml":    "vars:\n  - HOST: mod-default\ntasks:\n  show:\n    steps: []\n",
			},
			task:       "m:show",
			runtimeSet: map[string]string{"HOST": "caller"},
			want:       map[string]string{"HOST": "caller"},
		},
		{
			name: "given a module's own env: and vars:, when loaded, then neither reaches the root scope (why: a file scope never leaks to its importer)",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - m: ./mod.yml\ntasks: {}\n",
				"mod.yml":    "env:\n  - .env\nvars:\n  - LABEL: mod\ntasks:\n  show:\n    steps: []\n",
				".env":       "STAGE=dev\n",
			},
			wantAbsent: []string{"STAGE", "LABEL"},
		},
		{
			name: "given a nested module reading its parent module's env file, when the nested module's task starts, then both values reach it (why: a file scope covers its whole subtree)",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - p: ./parent.yml\ntasks: {}\n",
				"parent.yml": "env:\n  - parent.env\nmodules:\n  - c: ./child.yml\ntasks: {}\n",
				"parent.env": "REGION=eu\n",
				"child.yml":  "env:\n  - child.env\nvars:\n  - HOST: \"{{.REGION}}.example.com\"\ntasks:\n  show:\n    steps: []\n",
				"child.env":  "REGION=us\n",
			},
			task: "p:c:show",
			want: map[string]string{"REGION": "eu", "HOST": "eu.example.com"},
		},
		{
			name: "given a module env file marked secret, when a module task starts, then its vars are flagged secret",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - m: ./mod.yml\ntasks: {}\n",
				"mod.yml":    "env:\n  - .env:\n      secret: true\ntasks:\n  show:\n    steps: []\n",
				".env":       "TOKEN=hunter22\n",
			},
			task:        "m:show",
			want:        map[string]string{"TOKEN": "hunter22"},
			wantSecrets: []string{"TOKEN"},
		},
		{
			name: "given a module path templated on a root vars: entry, when loaded, then the path reads the importer's scope",
			files: map[string]string{
				"hobnob.yml": "vars:\n  - MOD: real\nmodules:\n  - m: \"./{{.MOD}}.yml\"\ntasks: {}\n",
				"real.yml":   "vars:\n  - WHO: real\ntasks:\n  show:\n    steps: []\n",
			},
			task: "m:show",
			want: map[string]string{"WHO": "real"},
		},
		{
			name: "given two modules importing each other, when loaded, then it errors naming the cycle",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - a: ./a.yml\ntasks: {}\n",
				"a.yml":      "modules:\n  - b: ./b.yml\ntasks: {}\n",
				"b.yml":      "modules:\n  - a: ./a.yml\ntasks: {}\n",
			},
			wantErr: "circular import",
		},
		{
			name: "given a module vars: entry that fails to evaluate, when loaded, then the error names the module and the block",
			files: map[string]string{
				"hobnob.yml": "modules:\n  - m: ./mod.yml\ntasks: {}\n",
				"mod.yml":    "vars:\n  - X: .NOPE.deeper\ntasks: {}\n",
			},
			wantErr: `module "m": vars: X`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			for key, val := range test.osEnv {
				t.Setenv(key, val)
			}
			cfg, err := config.ParseConfig(writeFiles(t, test.files))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			// Act
			root, fileScopes, err := scope.Load(context.Background(), cfg, test.cliVars, t.TempDir())
			got := root
			if err == nil && test.task != "" {
				task, ok := cfg.Tasks[test.task]
				if !ok {
					t.Fatalf("task %q not registered", test.task)
				}
				got = root.Copy()
				for key, val := range test.runtimeSet {
					got.Set(key, value.Str(val), false)
				}
				fileScopes.Apply(got, task.Cfg)
			}

			// Assert
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("got error %v, want one containing %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load error: %v", err)
			}
			for key, want := range test.want {
				if val, ok := got.Vars[key]; !ok || val.String() != want {
					t.Errorf("%s = %q (present %v), want %q", key, val.String(), ok, want)
				}
			}
			for _, key := range test.wantAbsent {
				if val, ok := got.Vars[key]; ok {
					t.Errorf("%s = %q, want absent", key, val.String())
				}
			}
			for _, key := range test.wantSecrets {
				if !got.Secrets[key] {
					t.Errorf("%s not flagged secret", key)
				}
			}
		})
	}
}
