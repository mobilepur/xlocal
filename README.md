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
xlocal init      # creates xlocal-config.json (asks a few questions)
xlocal           # shows what's missing, then translates interactively
```

On the first run xlocal asks for an Anthropic API key and stores it in the macOS keychain. Get one at [console.anthropic.com](https://console.anthropic.com).

You can also run `xlocal` from a parent folder — it discovers Xcode projects below and asks which one to use.

## Commands

| Command | What it does |
| --- | --- |
| `xlocal` | The main flow: analyze → select → translate → review → save |
| `xlocal status` | Read-only overview of missing translations (`--json` for CI) |
| `xlocal init` | Create the project config, proposing languages found in your catalogs |
| `xlocal keys add/list/remove/default` | Manage multiple API keys in the keychain |
| `xlocal config` | Set the global default model and key |
| `xlocal --dry-run` | Show exactly what would be translated, without API calls |
| `xlocal --key work` | Use a specific stored key for this run |

## Project configuration

`xlocal init` creates an `xlocal-config.json` in your project root. Check it in — it contains no secrets.

```json
{
  "targetLanguages": ["en", "de", "es", "fr"],
  "baseLanguages": ["en"],
  "untranslatableWords": ["MyAppName"],
  "formalLanguages": ["fr"],
  "model": "sonnet",
  "exclude": ["Vendor", "Generated"]
}
```

| Field | Meaning |
| --- | --- |
| `targetLanguages` | Languages xlocal keeps complete (required) |
| `baseLanguages` | Languages offered as a "base languages only" batch — typically your source language(s) |
| `untranslatableWords` | Brand/product names that must stay exactly as written |
| `formalLanguages` | Languages that address the user formally (Sie/Vous) |
| `model` | `sonnet` (default), `haiku`, `opus`, or a full model ID — overrides the global setting |
| `exclude` | Directory names or relative paths to skip |
| `customPrompt` | Optional: replace the built-in translation prompt (placeholders `{TARGET_LANGUAGE}`, `{KEY}`, `{SOURCE_TEXT}`) |

`Pods`, `DerivedData`, `node_modules`, `build`, `Carthage` and hidden directories are always skipped.

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
