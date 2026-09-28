package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseThemes(t *testing.T) {
	tests := []struct {
		name      string
		contents  string
		bodyColor string
	}{
		{"dark", "theme: dark\n", mustColor("yellow")},
		{"light", "theme: light\n", mustColor("94")},
		{"no-color", "theme: 'no-color'\n", ""},
		{"empty defaults to dark", "", mustColor("yellow")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte(tt.contents), nil)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if cfg.BodyColor != tt.bodyColor {
				t.Errorf("BodyColor = %q, want %q", cfg.BodyColor, tt.bodyColor)
			}
		})
	}
}

func TestParseColorOverride(t *testing.T) {
	cfg, err := Parse([]byte("theme: dark\nimmature-task-color: 'bold red'\n"), nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if want := mustColor("bold red"); cfg.ImmatureTaskColor != want {
		t.Errorf("ImmatureTaskColor = %q, want %q", cfg.ImmatureTaskColor, want)
	}
	if want := mustColor("yellow"); cfg.BodyColor != want {
		t.Errorf("BodyColor = %q, want the theme default %q", cfg.BodyColor, want)
	}
}

func TestParseCommands(t *testing.T) {
	cfg, err := Parse([]byte("editor-command: 'foo %FILENAME%'\npager-command: 'bar %PROMPT% %FILENAME%'\n"), nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.EditorCommand != "foo %FILENAME%" {
		t.Errorf("EditorCommand = %q", cfg.EditorCommand)
	}
	if cfg.PagerCommand != "bar %PROMPT% %FILENAME%" {
		t.Errorf("PagerCommand = %q", cfg.PagerCommand)
	}
}

func TestParseIgnoreTags(t *testing.T) {
	cfg, err := Parse([]byte("ignore-tags:\n - abc\n - def\n"), nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	want := []string{"abc", "def"}
	if !slices.Equal(cfg.IgnoreTags, want) {
		t.Errorf("IgnoreTags = %v, want %v", cfg.IgnoreTags, want)
	}
	if !cfg.HasIgnoredTag([]string{"xyz", "def"}) {
		t.Error("HasIgnoredTag did not find an ignored tag")
	}
	if cfg.HasIgnoredTag([]string{"xyz"}) {
		t.Error("HasIgnoredTag found a tag that is not ignored")
	}
}

func TestParseMonitorDisplayTime(t *testing.T) {
	tests := []struct {
		contents string
		want     bool
	}{
		{"monitor:\n  display-time: No\n", false},
		{"monitor:\n  display-time: Yes\n", true},
		{"", true},
	}

	for _, tt := range tests {
		cfg, err := Parse([]byte(tt.contents), nil)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", tt.contents, err)
		}
		if cfg.Monitor.DisplayTime != tt.want {
			t.Errorf("Parse(%q): DisplayTime = %v, want %v", tt.contents, cfg.Monitor.DisplayTime, tt.want)
		}
	}
}

func TestParseTrello(t *testing.T) {
	contents := "trello:\n" +
		"  api-key: \"foo\"\n" +
		"  token: \"bar\"\n" +
		"  tasks:\n" +
		"    \"board 1\":\n" +
		"      list1: \"tag1\"\n"

	cfg, err := Parse([]byte(contents), nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.Trello.APIKey != "foo" || cfg.Trello.Token != "bar" {
		t.Errorf("Trello credentials = %q/%q", cfg.Trello.APIKey, cfg.Trello.Token)
	}
	if got := cfg.Trello.Tasks["board 1"]["list1"]; got != "tag1" {
		t.Errorf("Trello board tag = %q, want %q", got, "tag1")
	}

	empty, err := Parse(nil, nil)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if empty.Trello.APIKey != "" || len(empty.Trello.Tasks) != 0 {
		t.Error("Trello settings were set without any configuration")
	}
}

// The secret file exists so that credentials can live apart from the rest of
// the configuration; what it sets must survive the merge.
func TestParseSecretMerges(t *testing.T) {
	cfg, err := Parse(
		[]byte("monitor:\n  display-time: Yes\n"),
		[]byte("trello:\n  api-key: \"foo\"\n  token: \"bar\"\n"),
	)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !cfg.Monitor.DisplayTime {
		t.Error("DisplayTime from the config file was lost")
	}
	if cfg.Trello.APIKey != "foo" || cfg.Trello.Token != "bar" {
		t.Errorf("Trello credentials = %q/%q", cfg.Trello.APIKey, cfg.Trello.Token)
	}
}

func TestParseRejectsBadConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{"unknown key", "not-a-real-key: 1\n"},
		{"unknown nested key", "monitor:\n  nope: 1\n"},
		{"unknown theme", "theme: chartreuse\n"},
		{"unknown color", "body-color: chartreuse\n"},
		{"not a mapping", "- a\n- b\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.contents), nil); err == nil {
				t.Error("Parse returned no error, want one")
			}
		})
	}
}

func TestLoadMissingFiles(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(filepath.Join(dir, "absent.yaml"), filepath.Join(dir, "absent.secret.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if want := mustColor("yellow"); cfg.BodyColor != want {
		t.Errorf("BodyColor = %q, want %q", cfg.BodyColor, want)
	}
	if len(cfg.IgnoreTags) != 0 {
		t.Errorf("IgnoreTags = %v, want none", cfg.IgnoreTags)
	}
}

func TestLoadReadsFiles(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "task.yaml")
	if err := os.WriteFile(configFile, []byte("theme: light\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configFile, filepath.Join(dir, "absent.yaml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if want := mustColor("94"); cfg.BodyColor != want {
		t.Errorf("BodyColor = %q, want %q", cfg.BodyColor, want)
	}
}
