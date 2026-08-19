package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	// given --version flag, when run, then output contains "hobnob" (why: version flag must identify the binary)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		os.Args = []string{"hobnob", "--version"}
		main()
		return
	}

	// Arrange
	cmd := exec.Command(os.Args[0], "-test.run=TestVersionFlag")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1")

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "hobnob") {
		t.Errorf("expected output to contain 'hobnob', got: %s", out)
	}
}

func TestHelpFlagShowsVersion(t *testing.T) {
	// given --help flag, when run, then first output line contains "hobnob" (why: version header anchors the help output)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "--help"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestHelpFlagShowsVersion")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	firstLine := strings.SplitN(string(out), "\n", 2)[0]
	if !strings.Contains(firstLine, "hobnob") {
		t.Errorf("expected version on first line of --help output, got: %q", firstLine)
	}
}

func TestNoArgsDefaultTaskRuns(t *testing.T) {
	// given taskfile with default task, when run with no args, then default task executes (why: default task is the no-arg entry point)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml")}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  default:\n    steps:\n      - run: echo hobnob-default-ran\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNoArgsDefaultTaskRuns")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "hobnob-default-ran") {
		t.Errorf("expected output to contain 'hobnob-default-ran', got: %s", out)
	}
}

func TestNoArgsNoDefaultTaskShowsTaskList(t *testing.T) {
	// given taskfile with no default task, when run with no args and no TTY, then lists tasks (why: non-interactive fallback for task selector)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml")}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  build:\n    steps:\n      - run: echo building\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNoArgsNoDefaultTaskShowsTaskList")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "Usage:") {
		t.Errorf("expected output to contain 'Usage:', got: %s", out)
	}
	if !strings.Contains(outStr, "build") {
		t.Errorf("expected output to contain task name 'build', got: %s", out)
	}
}

func TestNoArgsNoTasksShowsHelp(t *testing.T) {
	// given taskfile with zero tasks, when run with no args on a simulated TTY, then shows the same usage+list output as --help (why: regression — the interactive zero-task path used to print a bare "No tasks available." and skip usage/list output)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		isTerminalFn = func() bool { return true }
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml")}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNoArgsNoTasksShowsHelp")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir, "CI=")

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "Usage:") {
		t.Errorf("expected output to contain 'Usage:', got: %s", out)
	}
	if !strings.Contains(outStr, "No tasks found") {
		t.Errorf("expected output to contain 'No tasks found', got: %s", out)
	}
	if strings.Contains(outStr, "No tasks available.") {
		t.Errorf("stale message 'No tasks available.' should not appear, got: %s", out)
	}
}

func TestInternalTaskRejectedByCLI(t *testing.T) {
	// given _ prefixed task name, when run from CLI, then exits non-zero (why: internal tasks must not be directly invokable)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		os.Args = []string{"hobnob", "_helper"}
		main()
		return
	}

	// Arrange
	cmd := exec.Command(os.Args[0], "-test.run=TestInternalTaskRejectedByCLI")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1")

	// Act
	err := cmd.Run()

	// Assert
	if err == nil {
		t.Fatal("expected non-zero exit for internal task name, got success")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got: %v", err)
	}
}

func TestNamedTaskSuccessPrintsSuccessMessage(t *testing.T) {
	// given named task that succeeds, when run, then output contains ✓ and task name (why: success feedback must identify which task completed)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "deploy"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  deploy:\n    steps:\n      - run: echo ok\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNamedTaskSuccessPrintsSuccessMessage")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "✓") {
		t.Errorf("expected output to contain '✓', got: %s", out)
	}
	if !strings.Contains(outStr, "[deploy]") {
		t.Errorf("expected output to contain '[deploy]', got: %s", out)
	}
	if !strings.Contains(outStr, "done") {
		t.Errorf("expected output to contain 'done', got: %s", out)
	}
}

func TestNamedTaskFailurePrintsFailureMessage(t *testing.T) {
	// given named task that fails, when run, then output contains ✗ and task name (why: failure feedback must identify which task failed)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "deploy"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  deploy:\n    steps:\n      - run: exit 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNamedTaskFailurePrintsFailureMessage")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err == nil {
		t.Fatalf("expected failure, got success\noutput: %s", out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "✗") {
		t.Errorf("expected output to contain '✗', got: %s", out)
	}
	if !strings.Contains(outStr, "[deploy]") {
		t.Errorf("expected output to contain '[deploy]', got: %s", out)
	}
}

func TestDefaultTaskSuccessPrintsSuccessMessage(t *testing.T) {
	// given default task that succeeds, when run with no args, then output contains ✓ and "default" (why: success feedback applies to default task too)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml")}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  default:\n    steps:\n      - run: echo ok\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestDefaultTaskSuccessPrintsSuccessMessage")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "✓") {
		t.Errorf("expected output to contain '✓', got: %s", out)
	}
	if !strings.Contains(outStr, "[default]") {
		t.Errorf("expected output to contain '[default]', got: %s", out)
	}
	if !strings.Contains(outStr, "done") {
		t.Errorf("expected output to contain 'done', got: %s", out)
	}
}

func TestDefaultTaskFailurePrintsFailureMessage(t *testing.T) {
	// given default task that fails, when run with no args, then output contains ✗ and "default" (why: failure feedback applies to default task too)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml")}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  default:\n    steps:\n      - run: exit 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestDefaultTaskFailurePrintsFailureMessage")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err == nil {
		t.Fatalf("expected failure, got success\noutput: %s", out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "✗") {
		t.Errorf("expected output to contain '✗', got: %s", out)
	}
	if !strings.Contains(outStr, "[default]") {
		t.Errorf("expected output to contain '[default]', got: %s", out)
	}
}

func TestNoArgsDefaultTaskFailsFastWhenStdinNotTerminal(t *testing.T) {
	// given default task with a required get: and no CI/no TTY, when run with no args, then fails fast with the --no-input error instead of trying to open a prompt (why: regression — the no-args path used to compute noPrompts from CI alone, ignoring isTerminalFn)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml")}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  default:\n    steps:\n      - get: [FOO]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	env := []string{"CI=", "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR=" + dir}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CI=") {
			env = append(env, e)
		}
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestNoArgsDefaultTaskFailsFastWhenStdinNotTerminal")
	cmd.Env = env
	cmd.Stdin = nil // subprocess stdin defaults to /dev/null: never a terminal

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err == nil {
		t.Fatalf("expected failure, got success. output: %s", out)
	}
	if !strings.Contains(string(out), "--no-input: FOO requires input") {
		t.Errorf("expected fail-fast --no-input error, got: %s", out)
	}
}

func TestSelectFlagNoTTYFallsBackToList(t *testing.T) {
	// given --select flag with no TTY, when run, then falls back to task list output (why: non-interactive environments must not hang)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "--select"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  build:\n    info: Build it\n    steps:\n      - run: echo building\n  deploy:\n    steps:\n      - run: echo deploying\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestSelectFlagNoTTYFallsBackToList")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "build") {
		t.Errorf("expected output to contain 'build', got: %s", out)
	}
	if !strings.Contains(outStr, "deploy") {
		t.Errorf("expected output to contain 'deploy', got: %s", out)
	}
	if strings.Contains(outStr, "Usage:") {
		t.Errorf("--select should not show usage text, got: %s", out)
	}
}

func TestSelectFlagWithNoInputFallsBackToList(t *testing.T) {
	// given --select with --no-input, when run, then falls back to task list (why: --no-input must prevent interactive prompts)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "--select", "--no-input"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte("tasks:\n  build:\n    steps:\n      - run: echo building\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestSelectFlagWithNoInputFallsBackToList")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "build") {
		t.Errorf("expected output to contain 'build', got: %s", out)
	}
}

