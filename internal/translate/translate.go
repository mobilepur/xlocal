// Package translate builds translation prompts from analysis results and
// runs translations concurrently through a worker pool.
package translate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/MobilePur/xlocal/internal/analyze"
)

// DefaultTemplate is the built-in translation prompt. Projects can override
// it via the customPrompt field in xlocal-config.json; the placeholders
// {TARGET_LANGUAGE}, {KEY} and {SOURCE_TEXT} are replaced either way.
const DefaultTemplate = `You are a professional translator specializing in iOS app localization.

TASK: Translate the following iOS app string to {TARGET_LANGUAGE}.

CONTEXT:
- This is a localization key "{KEY}" in an iOS mobile application
- The text appears in a mobile app interface
- Keep the translation natural and user-friendly

SOURCE TEXT: "{SOURCE_TEXT}"

INSTRUCTIONS:
- Provide ONLY the translated text, no explanations or additional formatting
- Preserve any iOS-specific formatting like %@, %d, %lld placeholders exactly as they are
- CRITICAL: Keep any untranslatable words (brand names, product terms) EXACTLY as they appear in the source
- CAPITALIZATION: Follow the capitalization patterns of the existing translations for this key. Single words in UI contexts typically start with capital letters (e.g., "Cancel", "Save", "Delete").
- Keep the tone appropriate for a mobile app (usually friendly and concise)
- Ensure the translation fits well in mobile UI contexts (not too long)
- Use native, natural expressions that locals would actually use
- For button texts, use action-oriented language typical for {TARGET_LANGUAGE} UIs
- If the source text contains technical terms, keep them appropriate for the target market

TRANSLATION:`

// Options carries the project-level translation settings that shape the
// prompt.
type Options struct {
	Template            string // empty → DefaultTemplate
	UntranslatableWords []string
	FormalLanguages     []string
}

// BuildPrompt assembles the full prompt for one missing translation: the
// template plus contextual sections (key, plural instructions, developer
// comment, existing translations, untranslatable words, formality), inserted
// before the INSTRUCTIONS section so the model reads context first.
func BuildPrompt(m analyze.Missing, opts Options) string {
	template := opts.Template
	if template == "" {
		template = DefaultTemplate
	}

	var context strings.Builder

	context.WriteString(fmt.Sprintf("\nLOCALIZATION KEY: %s", m.Key))

	if m.IsPlural {
		context.WriteString("\n\nPLURAL FORM REQUIRED:")
		context.WriteString("\nThis is a PLURAL string that needs both singular (one) and plural (other) forms.")
		context.WriteString("\nFormat your response as: one: [singular form] | other: [plural form]")
		context.WriteString("\nExample: one: 1 document | other: %lld documents")
		context.WriteString("\nThe plural form should use %lld as the placeholder for the number.")
	}

	if m.Comment != "" {
		context.WriteString(fmt.Sprintf("\nDEVELOPER COMMENT: %s", m.Comment))
	}

	if len(m.Existing) > 0 {
		context.WriteString("\n\nEXISTING TRANSLATIONS FOR REFERENCE:")
		langs := make([]string, 0, len(m.Existing))
		for lang := range m.Existing {
			langs = append(langs, lang)
		}
		sort.Strings(langs)
		for _, lang := range langs {
			context.WriteString(fmt.Sprintf("\n%s: %s", strings.ToUpper(lang), m.Existing[lang]))
		}
	}

	if len(opts.UntranslatableWords) > 0 {
		context.WriteString("\n\nWORDS THAT MUST NOT BE TRANSLATED:")
		for _, word := range opts.UntranslatableWords {
			context.WriteString(fmt.Sprintf("\n- %s", word))
		}
		context.WriteString("\n\nThese are brand names and product terms that MUST remain EXACTLY as written above in ALL translations.")
	}

	if isFormal(m.TargetLanguage, opts.FormalLanguages) {
		context.WriteString(fmt.Sprintf("\n\nFORMALITY: Use FORMAL address (Sie/Vous) when referring to the user in %s.", strings.ToUpper(m.TargetLanguage)))
	} else {
		context.WriteString(fmt.Sprintf("\n\nFORMALITY: Use INFORMAL address (du/tu) when referring to the user in %s.", strings.ToUpper(m.TargetLanguage)))
	}

	prompt := template
	if insertAt := strings.Index(prompt, "INSTRUCTIONS:"); insertAt > 0 {
		prompt = prompt[:insertAt] + context.String() + "\n\n" + prompt[insertAt:]
	} else {
		prompt += context.String()
	}

	prompt = strings.ReplaceAll(prompt, "{TARGET_LANGUAGE}", m.TargetLanguage)
	prompt = strings.ReplaceAll(prompt, "{KEY}", m.Key)
	prompt = strings.ReplaceAll(prompt, "{SOURCE_TEXT}", m.SourceText)
	return prompt
}

func isFormal(lang string, formalLanguages []string) bool {
	for _, l := range formalLanguages {
		if l == lang {
			return true
		}
	}
	return false
}

// ParsePluralResponse splits a "one: ... | other: ..." model response into
// its plural forms. If the model ignored the format, the whole text becomes
// the "other" form and a singular is derived from it.
func ParsePluralResponse(response string) (one, other string) {
	for _, part := range strings.Split(response, " | ") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "one: ") {
			one = strings.TrimSpace(strings.TrimPrefix(part, "one: "))
		} else if strings.HasPrefix(part, "other: ") {
			other = strings.TrimSpace(strings.TrimPrefix(part, "other: "))
		}
	}

	if one == "" || other == "" {
		other = strings.TrimSpace(response)
		one = strings.ReplaceAll(other, "%lld", "1")
	}
	return one, other
}

// CleanTranslation trims whitespace and one pair of surrounding quotes that
// models sometimes add around the translated text.
func CleanTranslation(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		s = s[1 : len(s)-1]
	}
	return s
}

// Result is the outcome of translating one missing (key, language) pair.
type Result struct {
	Missing     analyze.Missing
	Translation string
	Err         error
}

// TranslateFunc produces the translation for one missing pair.
type TranslateFunc func(ctx context.Context, m analyze.Missing) (string, error)

// Run translates all items with up to concurrency parallel workers. Results
// are returned in input order. onResult, if set, is called serially as each
// item finishes — for live progress output. When ctx is canceled, queued
// items are not started and carry ctx.Err().
func Run(ctx context.Context, items []analyze.Missing, concurrency int, translate TranslateFunc, onResult func(Result)) []Result {
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([]Result, len(items))
	jobs := make(chan int)

	var callbackMu sync.Mutex
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				m := items[i]

				var r Result
				if err := ctx.Err(); err != nil {
					r = Result{Missing: m, Err: err}
				} else {
					translation, err := translate(ctx, m)
					r = Result{Missing: m, Translation: translation, Err: err}
				}
				results[i] = r

				if onResult != nil {
					callbackMu.Lock()
					onResult(r)
					callbackMu.Unlock()
				}
			}
		}()
	}

	for i := range items {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return results
}
