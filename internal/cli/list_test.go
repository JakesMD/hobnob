package cli

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"hobnob/internal/config"
)

func TestListTasks(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		wantLines  []string
		wantAbsent []string
	}{
		{
			name:    "given tasks with and without info, when listed, then shows names and rendered info templates",
			fixture: "testdata/task_info.yml",
			wantLines: []string{
				"adopt",
				"Adopt a cat from the shelter",
				"list_animals",
				"List all available animals",
				"no_info",
			},
		},
		{
			name:    "given tasks with get: params, when listed, then shows required params without parens and optional params with parens",
			fixture: "testdata/list_params.yml",
			wantLines: []string{
				"deploy",
				"Deploy the service to an environment",
				"ENV",
				"Target environment",
				"(PORT)",
				"Port to bind",
				"(RETRIES)",
				"Retry count",
				"rollback",
				"DEPLOY_ID",
				"ID of deployment to roll back",
				"CONFIRM",
				"no_params",
				"No parameters needed",
			},
			wantAbsent: []string{"required", "(default:"},
		},
		{
			name:    "given tasks with underscore-prefixed names, when listed, then internal tasks are absent (why: internal tasks must not be discoverable via CLI)",
			fixture: "testdata/internal_tasks.yml",
			wantLines: []string{
				"public",
				"another_public",
			},
			wantAbsent: []string{"_helper"},
		},
		{
			name:    "given no public tasks, when listed, then shows empty-state message with guide link (why: blank output is confusing for new users)",
			fixture: "testdata/no_public_tasks.yml",
			wantLines: []string{
				"No tasks found",
				"hobnob.yml",
				"GUIDE.md",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg, err := config.ParseConfig(test.fixture)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			scope, err := BuildScope(context.Background(), cfg.Vars, nil, nil, "/tmp/taskfile", "/tmp/invocation")
			if err != nil {
				t.Fatalf("scope error: %v", err)
			}
			var buf bytes.Buffer

			// Act
			if err := ListTasks(cfg, scope, &buf); err != nil {
				t.Fatalf("ListTasks error: %v", err)
			}

			// Assert
			output := buf.String()
			for _, want := range test.wantLines {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, output)
				}
			}
			for _, absent := range test.wantAbsent {
				if strings.Contains(output, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, output)
				}
			}
		})
	}
}

func TestCollectSelectableTasks(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantNames   []string
		wantInfos   []string
		absentNames []string
	}{
		{
			name:      "given tasks with and without info, when collected, then returns public tasks sorted with rendered info (why: selector needs display data)",
			fixture:   "testdata/task_info.yml",
			wantNames: []string{"adopt", "list_animals", "no_info"},
			wantInfos: []string{"Adopt a cat from the shelter", "List all available animals", ""},
		},
		{
			name:        "given internal tasks, when collected, then underscore-prefixed tasks excluded (why: internal tasks must not appear in selector)",
			fixture:     "testdata/internal_tasks.yml",
			wantNames:   []string{"another_public", "public"},
			absentNames: []string{"_helper"},
		},
		{
			name:      "given no public tasks, when collected, then returns empty slice (why: selector needs empty state)",
			fixture:   "testdata/no_public_tasks.yml",
			wantNames: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg, err := config.ParseConfig(test.fixture)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			scope, err := BuildScope(context.Background(), cfg.Vars, nil, nil, "/tmp/taskfile", "/tmp/invocation")
			if err != nil {
				t.Fatalf("scope error: %v", err)
			}

			// Act
			tasks := CollectSelectableTasks(cfg, scope)

			// Assert
			var gotNames []string
			for _, task := range tasks {
				gotNames = append(gotNames, task.Name)
			}
			if len(gotNames) == 0 {
				gotNames = []string{}
			}
			if !reflect.DeepEqual(gotNames, test.wantNames) {
				t.Errorf("names: got %v, want %v", gotNames, test.wantNames)
			}
			for i, wantInfo := range test.wantInfos {
				if i < len(tasks) && tasks[i].Info != wantInfo {
					t.Errorf("info[%d]: got %q, want %q", i, tasks[i].Info, wantInfo)
				}
			}
			for _, absent := range test.absentNames {
				for _, task := range tasks {
					if task.Name == absent {
						t.Errorf("should not contain %q", absent)
					}
				}
			}
		})
	}
}
