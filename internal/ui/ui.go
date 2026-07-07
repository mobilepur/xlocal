// Package ui bundles the terminal styling helpers shared by all commands.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	Title   = lipgloss.NewStyle().Bold(true)
	Dim     = lipgloss.NewStyle().Faint(true)
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	Warn    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	Error   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	Accent  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

var flags = map[string]string{
	"ar":    "🇸🇦",
	"de":    "🇩🇪",
	"en":    "🇬🇧",
	"en-GB": "🇬🇧",
	"en-US": "🇺🇸",
	"es":    "🇪🇸",
	"fr":    "🇫🇷",
	"it":    "🇮🇹",
	"ja":    "🇯🇵",
	"ko":    "🇰🇷",
	"nl":    "🇳🇱",
	"pl":    "🇵🇱",
	"pt-BR": "🇧🇷",
	"pt-PT": "🇵🇹",
	"ru":    "🇷🇺",
	"sv":    "🇸🇪",
	"tr":    "🇹🇷",
	"uk":    "🇺🇦",
	"zh":    "🇨🇳",
}

// Flag returns the flag emoji for a language code, or a neutral flag for
// unknown codes.
func Flag(lang string) string {
	if flag, ok := flags[lang]; ok {
		return flag
	}
	if flag, ok := flags[strings.SplitN(lang, "-", 2)[0]]; ok {
		return flag
	}
	return "🏳️"
}

// Lang renders a language code with its flag, e.g. "🇩🇪 DE".
func Lang(lang string) string {
	return fmt.Sprintf("%s %s", Flag(lang), strings.ToUpper(lang))
}
