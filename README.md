# xlocal

Translate missing strings in Xcode String Catalogs (`.xcstrings`) using the Anthropic API — right from your terminal.

- 🔍 Scans your project for String Catalogs and shows what's missing, per language
- 🤖 Translates with Claude, using your existing translations, developer comments, and plural forms as context
- 🔐 API keys live in the macOS keychain — never in your repo
- ✍️ Writes catalogs in Xcode's exact format, so diffs stay minimal
- 🧢 Keeps brand names untranslated and warns when the model slips

## Install

```sh
brew install mobilepur/tap/xlocal
```

Or build from source (Go 1.22+):

```sh
go install github.com/MobilePur/xlocal@latest
```

## Quickstart

```sh
cd /path/to/YourApp
xlocal init      # creates a config skeleton and checks your API key setup
xlocal           # shows what's missing, then translates interactively
```

`init` prefills the target languages found in your catalogs and leaves everything else empty — fill in the rest with `xlocal config`. If no API key is stored yet, `init` offers to add one to the macOS keychain. Get one at [console.anthropic.com](https://console.anthropic.com).

You can also run `xlocal` from a parent folder — it discovers Xcode projects below and asks which one to use.

## Commands

| Command | What it does |
| --- | --- |
| `xlocal` | The main flow: analyze → select → translate → review → save |
| `xlocal status` | Read-only overview of missing translations (`--json` for CI) |
| `xlocal init` | Set up the project: config skeleton (languages prefilled) + API key check |
| `xlocal config` | Open the project config in `$VISUAL`/`$EDITOR` (falls back to vim) |
| `xlocal config global` | Set the global default model and key |
| `xlocal keys add/list/remove/default` | Manage multiple API keys in the keychain |
| `xlocal deintegrate` | Remove xlocal from the project: delete the config in the current folder |
| `xlocal conventions` | Show the catalog conventions xlocal relies on |
| `xlocal --dry-run` | Show exactly what would be translated, without API calls |
| `xlocal --key work` | Use a specific stored key for this run |

## Project configuration

`xlocal init` creates an `xlocal-config.json` in your project root. Check it in — it contains no secrets.

```json
{
  "strategy": "merge",
  "targetLanguages": ["en", "de", "es", "fr"],
  "baseLanguages": ["en"],
  "untranslatableWords": ["MyAppName"],
  "formalLanguages": ["fr"],
  "model": "sonnet",
  "exclude": ["Vendor", "Generated"],
  "excludeKeys": ["legal.disclaimer"]
}
```

| Field | Meaning |
| --- | --- |
| `strategy` | `merge` (default) inherits and refines parent fields; `override` uses only this config for its subtree |
| `targetLanguages` | Languages xlocal keeps complete (required) |
| `baseLanguages` | Languages offered as a "base languages only" batch — typically your source language(s) |
| `untranslatableWords` | Brand/product names that must stay exactly as written |
| `formalLanguages` | Languages that address the user formally (Sie/Vous) |
| `model` | `sonnet` (default), `haiku`, `opus`, or a full model ID — overrides the global setting |
| `exclude` | Directory names or relative paths to skip |
| `excludeKeys` | String keys xlocal never translates |
| `customPrompt` | Optional: replace the built-in translation prompt (placeholders `{TARGET_LANGUAGE}`, `{KEY}`, `{SOURCE_TEXT}`) |

`Pods`, `DerivedData`, `node_modules`, `build`, `Carthage` and hidden directories are always skipped. Strings marked **Don't Translate** in Xcode (`shouldTranslate: false`) and keys without any translatable source text are skipped as well.

### Nested configuration (per folder)

Like Git, xlocal supports a config in **any** folder. The top-level `strategy` field controls how a nested config relates to the configs above it:

- **`merge` is the default.** The root config and every nested merge config down to a catalog's folder are layered together. A nested config may be partial: fields it leaves out are inherited, while `targetLanguages`, `baseLanguages`, `formalLanguages` and `customPrompt` replace the inherited field when set.
- **`override` starts fresh.** Configs above it are ignored for its entire subtree. The override config must therefore provide its own `targetLanguages`; it may also select its own model.
- **`untranslatableWords` and `excludeKeys` accumulate.** A subfolder adds its own brand names / excluded keys on top of the inherited ones (deduplicated union).
- **`model` stays inherited while merging.** A nested merge config cannot switch models; an override config can.
- **`exclude` is scoped.** An `exclude` entry only skips directories within the subtree of the config that declares it.

The project root is anchored at the **topmost config in the active merge chain**; an `override` config becomes the root when running inside its subtree. Create a nested config with `xlocal init` inside a subfolder — it detects the config above and creates a partial merge config that inherits from it.

## Conventions

xlocal builds its translation prompts from your catalog — the quality of what goes in decides the quality of what comes out. Also available in the terminal via `xlocal conventions`.

1. **English is the source language.** Write source strings in English; all other languages are translated from it.
2. **Developer comments are written in English.** The comment of a key is sent to the model — it's your main way to give context: what the string is ("button label", "empty-state title") and where it appears.
3. **Existing translations are the reference.** Every existing translation of a key is included in the prompt, so new languages stay consistent with your established terminology.
4. **Brand and product names go into `untranslatableWords`.** xlocal instructs the model to keep them exactly as written and warns you when one was translated anyway.

## How translations work

For every missing (key, language) pair, xlocal builds a prompt containing the source text, the localization key, the developer comment, all existing translations of that key, your untranslatable words, and formality instructions. Plural strings are translated as proper `one`/`other` variations. Four translations run in parallel; you review a summary (including brand-word warnings) before anything is written.

Files are written in the exact format Xcode's String Catalog editor produces — byte-identical round trips, minimal diffs.

## Development

```sh
go test ./...
go build
```

Releases are built with [GoReleaser](https://goreleaser.com) via GitHub Actions on tag push (`v*`) and published to `mobilepur/homebrew-tap`.

## License

[MIT](LICENSE)
