package config

import (
	"testing"
)

func TestLoadModules_InternalPrefix(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/modules_parent.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Act
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	// Assert — _farm tasks registered, hidden, callable
	tests := []struct {
		taskName   string
		wantHidden bool
	}{
		{"_farm:milk_cow", true},
		{"_farm:feed_cow", true},
	}
	for _, tc := range tests {
		t.Run("given internal module, when loaded, then task "+tc.taskName+" is hidden (why: _ prefix blocks --list)", func(t *testing.T) {
			// Arrange (tc)

			// Act
			task, ok := cfg.Tasks[tc.taskName]

			// Assert
			if !ok {
				t.Fatalf("task %q not registered", tc.taskName)
			}
			if task.Hidden != tc.wantHidden {
				t.Errorf("Hidden: got %v, want %v", task.Hidden, tc.wantHidden)
			}
			if task.Cfg == nil {
				t.Error("Cfg should not be nil for module task")
			}
		})
	}
}

func TestLoadModules_ShowFilter(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/modules_parent.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Act
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	// Assert — show:[clean,fix] means die is not registered under yard prefix
	t.Run("given show:[clean,fix], when loaded, then yard:die not registered (why: show filter excludes die)", func(t *testing.T) {
		// Arrange (none extra)

		// Act
		_, ok := cfg.Tasks["yard:die"]

		// Assert
		if ok {
			t.Error("yard:die should not be registered due to show filter")
		}
	})

	t.Run("given show:[clean,fix], when loaded, then yard:clean is registered", func(t *testing.T) {
		// Arrange (none)

		// Act
		_, ok := cfg.Tasks["yard:clean"]

		// Assert
		if !ok {
			t.Error("yard:clean should be registered")
		}
	})
}

func TestLoadModules_HideFilter(t *testing.T) {
	// Arrange — use a dedicated fixture with only hide (no show)
	cfg := &ConfigFile{
		Tasks:       make(map[string]Task),
		TaskfileDir: "testdata",
		Modules: []ModuleEntry{
			{
				Prefix:    "yard",
				FileTmpl:  "modules_yard.yml",
				HideTmpls: []string{"die"},
			},
		},
	}

	// Act
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	tests := []struct {
		name      string
		taskName  string
		wantFound bool
	}{
		{
			name:      "given hide:[die], when loaded, then yard:die not registered (why: hide filter excludes it)",
			taskName:  "yard:die",
			wantFound: false,
		},
		{
			name:      "given hide:[die], when loaded, then yard:clean registered (why: not in hide list)",
			taskName:  "yard:clean",
			wantFound: true,
		},
		{
			name:      "given hide:[die], when loaded, then yard:fix registered (why: not in hide list)",
			taskName:  "yard:fix",
			wantFound: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange (tc)

			// Act
			_, ok := cfg.Tasks[tc.taskName]

			// Assert
			if ok != tc.wantFound {
				t.Errorf("task %q found=%v, want %v", tc.taskName, ok, tc.wantFound)
			}
		})
	}
}

func TestLoadModules_Flatten(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/modules_parent.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Act
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	tests := []struct {
		name           string
		flatName       string
		prefixedName   string
		wantFlatFound  bool
		wantPrefixHide bool
	}{
		{
			name:           "given flatten:true, when loaded, then clean registered without prefix (why: flatten exposes flat alias)",
			flatName:       "clean",
			prefixedName:   "yard:clean",
			wantFlatFound:  true,
			wantPrefixHide: true,
		},
		{
			name:           "given flatten:true, when loaded, then fix registered without prefix",
			flatName:       "fix",
			prefixedName:   "yard:fix",
			wantFlatFound:  true,
			wantPrefixHide: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange (tc)

			// Act
			flatTask, flatOk := cfg.Tasks[tc.flatName]
			prefixTask, prefixOk := cfg.Tasks[tc.prefixedName]

			// Assert
			if flatOk != tc.wantFlatFound {
				t.Errorf("flat task %q found=%v, want %v", tc.flatName, flatOk, tc.wantFlatFound)
			}
			if !prefixOk {
				t.Fatalf("prefixed task %q not found", tc.prefixedName)
			}
			if prefixTask.Hidden != tc.wantPrefixHide {
				t.Errorf("prefixed task Hidden=%v, want %v", prefixTask.Hidden, tc.wantPrefixHide)
			}
			if flatOk && flatTask.Hidden {
				t.Errorf("flat task %q should not be hidden", tc.flatName)
			}
		})
	}
}

func TestLoadModules_ModuleCfgIsolation(t *testing.T) {
	// Arrange — module task must carry its own Cfg for sub-call isolation
	cfg, err := ParseConfig("testdata/modules_parent.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Act
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	// Assert
	t.Run("given module task, when loaded, then Cfg points to module config (why: prevents module tasks calling parent tasks)", func(t *testing.T) {
		// Arrange
		task, ok := cfg.Tasks["_farm:milk_cow"]
		if !ok {
			t.Fatal("_farm:milk_cow not found")
		}

		// Act (none)

		// Assert
		if task.Cfg == nil {
			t.Fatal("Cfg should not be nil")
		}
		_, hasMilkCow := task.Cfg.Tasks["milk_cow"]
		if !hasMilkCow {
			t.Error("module Cfg should contain milk_cow")
		}
		_, hasDeploy := task.Cfg.Tasks["deploy"]
		if hasDeploy {
			t.Error("module Cfg must not contain parent task 'deploy' (why: isolation)")
		}
	})
}

func TestLoadModules_TemplatePath(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/modules_template_path.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Act — default kicks in since FARM_FILE is not set
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	// Assert
	t.Run("given template file path with default, when scope has no FARM_FILE, then default used (why: template evaluated against scope)", func(t *testing.T) {
		// Arrange (none)

		// Act
		_, ok := cfg.Tasks["farm:milk_cow"]

		// Assert
		if !ok {
			t.Error("farm:milk_cow should be registered via default path")
		}
	})
}

func TestLoadModules_FlattenCollision(t *testing.T) {
	// Arrange — parent already has a task named 'clean'; flatten should not override it
	cfg := &ConfigFile{
		Tasks: map[string]Task{
			"clean": {Info: "parent clean"},
		},
		TaskNames:   []string{"clean"},
		TaskfileDir: "testdata",
		Modules: []ModuleEntry{
			{
				Prefix:      "yard",
				FileTmpl:    "modules_yard.yml",
				FlattenTmpl: "true",
			},
		},
	}

	// Act
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	// Assert
	t.Run("given flatten collision, when loaded, then parent task not overridden (why: parent tasks take precedence)", func(t *testing.T) {
		// Arrange (none)

		// Act
		task := cfg.Tasks["clean"]

		// Assert
		if task.Info != "parent clean" {
			t.Errorf("parent clean overridden; got info %q", task.Info)
		}
		// prefixed version should still be registered (and visible, since flat failed)
		yardClean, ok := cfg.Tasks["yard:clean"]
		if !ok {
			t.Fatal("yard:clean not registered")
		}
		if yardClean.Hidden {
			t.Error("yard:clean should not be hidden when flat alias collided with parent task")
		}
	})
}

func TestLoadModules_SubdirRelativePath(t *testing.T) {
	// Arrange — taskfile in testdata/, module in testdata/subdir/
	cfg, err := ParseConfig("testdata/modules_parent.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// Inject a module entry pointing into a subdirectory.
	cfg.Modules = []ModuleEntry{
		{Prefix: "sub", FileTmpl: "subdir/modules_sub.yml"},
	}

	// Act
	if err := LoadModules(cfg, map[string]string{}); err != nil {
		t.Fatalf("LoadModules error: %v", err)
	}

	tests := []struct {
		name     string
		taskName string
		wantInfo string
	}{
		{
			name:     "given subdir module path, when loaded, then sub:hello registered (why: path resolves relative to taskfile dir)",
			taskName: "sub:hello",
			wantInfo: "Say hello",
		},
		{
			name:     "given subdir module path, when loaded, then sub:world registered (why: path resolves relative to taskfile dir)",
			taskName: "sub:world",
			wantInfo: "Say world",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			task, ok := cfg.Tasks[tc.taskName]

			// Assert
			if !ok {
				t.Fatalf("task %q not found", tc.taskName)
			}
			if task.Info != tc.wantInfo {
				t.Errorf("got info %q, want %q", task.Info, tc.wantInfo)
			}
		})
	}
}
