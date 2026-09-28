// Package config reads the task application's YAML configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jmaslak/go-task/internal/ansi"
)

// Config holds the settings that control the appearance and behavior of the
// application.
type Config struct {
	BodyColor              string
	HeaderAlertColor       string
	HeaderNormalColor      string
	HeaderSeperatorColor   string
	HeaderTitleColor       string
	ImmatureTaskColor      string
	NotDisplayedTodayColor string
	PromptBoldColor        string
	PromptColor            string
	PromptInfoColor        string
	TagColor               string
	Reset                  string

	// IgnoreTags lists tags whose tasks are hidden from the default task
	// listing.
	IgnoreTags []string

	PagerCommand  string
	EditorCommand string

	Monitor Monitor
	Trello  Trello
}

// Monitor holds the settings for the monitor command.
type Monitor struct {
	DisplayTime bool
}

// Trello holds the credentials and board mapping used by the trello-sync
// command.
type Trello struct {
	APIKey  string
	Token   string
	BaseURL string
	// Tasks maps a board name to the lists on that board that should be
	// synced, and each of those lists to the tag given to its tasks.
	Tasks map[string]map[string]string
}

const (
	defaultPagerCommand  = "less -RFX -P%PROMPT% -- %FILENAME%"
	defaultEditorCommand = "nano -b -r 72 -s ispell +3,1 %FILENAME%"
)

// DefaultPaths returns the locations of the configuration file and of the
// companion file holding secrets.
func DefaultPaths() (configFile, secretFile string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".task.yaml", ".task.secret.yaml"
	}
	return filepath.Join(home, ".task.yaml"), filepath.Join(home, ".task.secret.yaml")
}

// Load reads the configuration from the given files. Files that do not exist
// are treated as empty, so a missing configuration yields the defaults.
func Load(configFile, secretFile string) (*Config, error) {
	contents, err := readIfPresent(configFile)
	if err != nil {
		return nil, err
	}
	secret, err := readIfPresent(secretFile)
	if err != nil {
		return nil, err
	}

	return Parse(contents, secret)
}

// Parse builds a configuration from the contents of the configuration file
// and of the secret file. Settings in the secret file win.
func Parse(contents, secret []byte) (*Config, error) {
	var f file
	if err := decode(contents, &f); err != nil {
		return nil, fmt.Errorf("config file: %w", err)
	}
	if err := decode(secret, &f); err != nil {
		return nil, fmt.Errorf("secret file: %w", err)
	}

	return f.config()
}

// NoColor returns the default configuration with all colorization disabled.
func NoColor() *Config {
	return &Config{
		PagerCommand:  defaultPagerCommand,
		EditorCommand: defaultEditorCommand,
		Monitor:       Monitor{DisplayTime: true},
		Trello:        Trello{Tasks: map[string]map[string]string{}},
	}
}

// HasIgnoredTag reports whether any of tags is configured to be ignored.
func (c *Config) HasIgnoredTag(tags []string) bool {
	for _, tag := range tags {
		if slices.Contains(c.IgnoreTags, tag) {
			return true
		}
	}
	return false
}

// file mirrors the YAML document. Every scalar is a pointer so that an absent
// key can be told apart from one that was explicitly set, which is what makes
// the secret file able to override - and only override - what it names.
type file struct {
	Theme                  *string `yaml:"theme"`
	BodyColor              *string `yaml:"body-color"`
	HeaderAlertColor       *string `yaml:"header-alert-color"`
	HeaderNormalColor      *string `yaml:"header-normal-color"`
	HeaderSeperatorColor   *string `yaml:"header-seperator-color"`
	HeaderTitleColor       *string `yaml:"header-title-color"`
	ImmatureTaskColor      *string `yaml:"immature-task-color"`
	NotDisplayedTodayColor *string `yaml:"not-displayed-today-color"`
	PromptBoldColor        *string `yaml:"prompt-bold-color"`
	PromptColor            *string `yaml:"prompt-color"`
	PromptInfoColor        *string `yaml:"prompt-info-color"`
	TagColor               *string `yaml:"tag-color"`
	Reset                  *string `yaml:"reset"`

	IgnoreTags    []string `yaml:"ignore-tags"`
	PagerCommand  *string  `yaml:"pager-command"`
	EditorCommand *string  `yaml:"editor-command"`

	Monitor *monitorFile `yaml:"monitor"`
	Trello  *trelloFile  `yaml:"trello"`
}

type monitorFile struct {
	DisplayTime *bool `yaml:"display-time"`
}

type trelloFile struct {
	APIKey  *string                      `yaml:"api-key"`
	Token   *string                      `yaml:"token"`
	BaseURL *string                      `yaml:"base-url"`
	Tasks   map[string]map[string]string `yaml:"tasks"`
}

// config turns the parsed document into a Config.
func (f *file) config() (*Config, error) {
	c := NoColor()

	theme := "dark"
	if f.Theme != nil {
		theme = strings.ToLower(*f.Theme)
	}
	switch theme {
	case "dark":
		setDarkTheme(c)
	case "light":
		setLightTheme(c)
	case "no-color":
		// NoColor is already the starting point.
	default:
		return nil, fmt.Errorf("unknown theme type: %s", theme)
	}

	colors := []struct {
		spec *string
		dst  *string
	}{
		{f.BodyColor, &c.BodyColor},
		{f.HeaderAlertColor, &c.HeaderAlertColor},
		{f.HeaderNormalColor, &c.HeaderNormalColor},
		{f.HeaderSeperatorColor, &c.HeaderSeperatorColor},
		{f.HeaderTitleColor, &c.HeaderTitleColor},
		{f.ImmatureTaskColor, &c.ImmatureTaskColor},
		{f.NotDisplayedTodayColor, &c.NotDisplayedTodayColor},
		{f.PromptBoldColor, &c.PromptBoldColor},
		{f.PromptColor, &c.PromptColor},
		{f.PromptInfoColor, &c.PromptInfoColor},
		{f.TagColor, &c.TagColor},
		{f.Reset, &c.Reset},
	}
	for _, color := range colors {
		if color.spec == nil {
			continue
		}
		escape, err := Color(*color.spec)
		if err != nil {
			return nil, err
		}
		*color.dst = escape
	}

	if f.IgnoreTags != nil {
		c.IgnoreTags = f.IgnoreTags
	}
	if f.PagerCommand != nil {
		c.PagerCommand = *f.PagerCommand
	}
	if f.EditorCommand != nil {
		c.EditorCommand = *f.EditorCommand
	}

	if f.Monitor != nil && f.Monitor.DisplayTime != nil {
		c.Monitor.DisplayTime = *f.Monitor.DisplayTime
	}

	if f.Trello != nil {
		if f.Trello.APIKey != nil {
			c.Trello.APIKey = *f.Trello.APIKey
		}
		if f.Trello.Token != nil {
			c.Trello.Token = *f.Trello.Token
		}
		if f.Trello.BaseURL != nil {
			c.Trello.BaseURL = *f.Trello.BaseURL
		}
		if f.Trello.Tasks != nil {
			c.Trello.Tasks = f.Trello.Tasks
		}
	}

	return c, nil
}

// Color renders a color specification from the configuration file. Attributes
// are reset first, so each specification stands on its own.
func Color(spec string) (string, error) {
	fields := strings.Fields(spec)
	if !slices.Contains(fields, "reset") {
		fields = append([]string{"reset"}, fields...)
	}
	return ansi.Color(strings.Join(fields, " "))
}

func setDarkTheme(c *Config) {
	c.BodyColor = mustColor("yellow")
	c.HeaderAlertColor = mustColor("bold red")
	c.HeaderNormalColor = mustColor("bold yellow")
	c.HeaderSeperatorColor = mustColor("red")
	c.HeaderTitleColor = mustColor("bold green")
	c.ImmatureTaskColor = mustColor("yellow")
	c.NotDisplayedTodayColor = mustColor("yellow")
	c.PromptBoldColor = mustColor("bold green")
	c.PromptColor = mustColor("bold cyan")
	c.PromptInfoColor = mustColor("cyan")
	c.TagColor = mustColor("red")
	c.Reset = ansi.Reset()
}

func setLightTheme(c *Config) {
	c.BodyColor = mustColor("94")
	c.HeaderAlertColor = mustColor("red")
	c.HeaderNormalColor = mustColor("94")
	c.HeaderSeperatorColor = mustColor("red")
	c.HeaderTitleColor = mustColor("28")
	c.ImmatureTaskColor = mustColor("yellow")
	c.NotDisplayedTodayColor = mustColor("yellow")
	c.PromptBoldColor = mustColor("green")
	c.PromptColor = mustColor("cyan")
	c.PromptInfoColor = mustColor("cyan")
	c.TagColor = mustColor("28")
	c.Reset = ansi.Reset()
}

// mustColor renders a color specification that is known good at compile time.
func mustColor(spec string) string {
	escape, err := Color(spec)
	if err != nil {
		panic(err)
	}
	return escape
}

// decode parses YAML into dst, rejecting keys that the application does not
// understand. Decoding an empty document leaves dst untouched.
func decode(data []byte, dst *file) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// readIfPresent reads a file, reporting a missing file as empty contents.
func readIfPresent(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}
