package ansi

import "testing"

func TestColor(t *testing.T) {
	tests := []struct {
		spec string
		want string
	}{
		{"", ""},
		{"reset", "\x1b[0m"},
		{"yellow", "\x1b[33m"},
		{"reset bold red", "\x1b[0;1;31m"},
		{"RESET Bold Green", "\x1b[0;1;32m"},
		{"reset 94", "\x1b[0;38;5;94m"},
		{"on_blue", "\x1b[44m"},
		{"on_28", "\x1b[48;5;28m"},
		{"bright_cyan", "\x1b[96m"},
	}

	for _, tt := range tests {
		got, err := Color(tt.spec)
		if err != nil {
			t.Errorf("Color(%q) returned error: %v", tt.spec, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Color(%q) = %q, want %q", tt.spec, got, tt.want)
		}
	}
}

func TestColorRejectsUnknownAttributes(t *testing.T) {
	for _, spec := range []string{"chartreuse", "256", "-1", "on_999"} {
		if _, err := Color(spec); err == nil {
			t.Errorf("Color(%q) returned no error, want one", spec)
		}
	}
}
