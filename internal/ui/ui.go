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
	"hi":    "🇮🇳",
	"id":    "🇮🇩",
	"it":    "🇮🇹",
	"ja":    "🇯🇵",
	"ko":    "🇰🇷",
	"nl":    "🇳🇱",
	"pl":    "🇵🇱",
	"pt-BR": "🇧🇷",
	"pt-PT": "🇵🇹",
	"ru":    "🇷🇺",
	"sv":    "🇸🇪",
	"th":    "🇹🇭",
	"tr":    "🇹🇷",
	"uk":    "🇺🇦",
	"vi":    "🇻🇳",
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

// LangPadded renders a language code like Lang, padding the code to width so
// that whatever follows lines up across languages of different lengths
// (e.g. "DE" vs "ZH-HANS").
func LangPadded(lang string, width int) string {
	return fmt.Sprintf("%s %-*s", Flag(lang), width, strings.ToUpper(lang))
}

// MaxLangWidth returns the length of the longest language code in langs,
// for use as the width argument of LangPadded.
func MaxLangWidth(langs []string) int {
	width := 0
	for _, lang := range langs {
		if len(lang) > width {
			width = len(lang)
		}
	}
	return width
}
