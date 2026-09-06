// Package ansi renders ANSI SGR escape sequences from the human readable
// color specifications used in the task configuration file.
//
// The accepted syntax matches Raku's Terminal::ANSIColor module, so
// configuration files written for App::Tasks keep working: a specification is
// a space separated list of attributes such as "bold red", "on_blue", or a
// number selecting from the 256 color palette ("94").
package ansi

import (
	"fmt"
	"strconv"
	"strings"
)

// attributes maps an attribute name to its SGR parameter.
var attributes = map[string]int{
	"reset":      0,
	"bold":       1,
	"dark":       2,
	"faint":      2,
	"italic":     3,
	"underline":  4,
	"underscore": 4,
	"blink":      5,
	"reverse":    7,
	"inverse":    7,
	"concealed":  8,

	"black":   30,
	"red":     31,
	"green":   32,
	"yellow":  33,
	"blue":    34,
	"magenta": 35,
	"cyan":    36,
	"white":   37,
	"default": 39,

	"bright_black":   90,
	"bright_red":     91,
	"bright_green":   92,
	"bright_yellow":  93,
	"bright_blue":    94,
	"bright_magenta": 95,
	"bright_cyan":    96,
	"bright_white":   97,

	"on_black":   40,
	"on_red":     41,
	"on_green":   42,
	"on_yellow":  43,
	"on_blue":    44,
	"on_magenta": 45,
	"on_cyan":    46,
	"on_white":   47,
	"on_default": 49,

	"on_bright_black":   100,
	"on_bright_red":     101,
	"on_bright_green":   102,
	"on_bright_yellow":  103,
	"on_bright_blue":    104,
	"on_bright_magenta": 105,
	"on_bright_cyan":    106,
	"on_bright_white":   107,
}

// Color returns the escape sequence that turns on every attribute named in
// spec. An empty spec yields an empty string, which disables colorization.
func Color(spec string) (string, error) {
	fields := strings.Fields(strings.ToLower(spec))
	if len(fields) == 0 {
		return "", nil
	}

	params := make([]string, 0, len(fields))
	for _, name := range fields {
		param, err := parameter(name)
		if err != nil {
			return "", err
		}
		params = append(params, param)
	}

	return "\x1b[" + strings.Join(params, ";") + "m", nil
}

// Reset returns the escape sequence that clears all attributes.
func Reset() string {
	return "\x1b[0m"
}

// parameter renders a single attribute name as SGR parameters.
func parameter(name string) (string, error) {
	if param, ok := attributes[name]; ok {
		return strconv.Itoa(param), nil
	}

	// Bare numbers, optionally prefixed with "on_", select from the 256
	// color palette.
	digits, background := strings.CutPrefix(name, "on_")
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 || n > 255 {
		return "", fmt.Errorf("unknown color attribute %q", name)
	}
	if background {
		return "48;5;" + strconv.Itoa(n), nil
	}
	return "38;5;" + strconv.Itoa(n), nil
}
