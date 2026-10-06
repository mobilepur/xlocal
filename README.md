# xlocal

Translate missing strings in Xcode String Catalogs (`.xcstrings`) and Android XML resources using the Anthropic API — right from your terminal.

Android XML support is experimental. See [Android support](#android-support-experimental) for setup, local testing and current limitations.

- 🔍 Scans Xcode String Catalogs and configured Android resource folders, showing what's missing per language
- 🤖 Translates with Claude, using your existing translations, developer comments, and plural forms as context
- 🔐 API keys live in the macOS keychain — never in your repo
- ✍️ Writes Xcode catalogs in their original format and adds missing Android translations while preserving existing XML
- 🧢 Keeps brand names untranslated and warns when the model slips

## Install

```sh
brew install mobilepur/tap/xlocal
```

Or install from source (Go 1.25.14+, as specified in `go.mod`):

```sh
go install github.com/MobilePur/xlocal@latest
```

## Quickstart

```sh
cd /path/to/YourApp
xlocal init      # creates a config skeleton and checks your API key setup
xlocal           # shows what's missing, then translates interactively
```

`init` prefills languages found in your localization resources. For Android, it also selects standard `*/src/main/res` folders and sets the source language to English. Review the config and add any new target languages with `xlocal config`. If no API key is stored yet, `init` offers to add one to the macOS keychain. Get one at [console.anthropic.com](https://console.anthropic.com).

You can also run `xlocal` from a parent folder — it discovers Xcode and Android projects below and asks which one to use. Android projects are identified by `settings.gradle` or `settings.gradle.kts`.

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
| `androidResources` | Android resource roots, relative to the config that declares them (for example `["app/src/main/res"]`); required to include Android resources |
| `androidSourceLanguage` | Language of Android's unqualified `values` resources; defaults to `en` |
| `customPrompt` | Optional: replace the built-in translation prompt (placeholders `{TARGET_LANGUAGE}`, `{KEY}`, `{SOURCE_TEXT}`) |

`Pods`, `DerivedData`, `node_modules`, `build`, `Carthage` and hidden directories are always skipped. Strings marked **Don't Translate** in Xcode (`shouldTranslate: false`), Android resources with `translatable="false"`, and keys without any translatable source text are skipped as well.

### Nested configuration (per folder)

Like Git, xlocal supports a config in **any** folder. The top-level `strategy` field controls how a nested config relates to the configs above it:

- **`merge` is the default.** The root config and every nested merge config down to a catalog's folder are layered together. A nested config may be partial: fields it leaves out are inherited, while `targetLanguages`, `baseLanguages`, `formalLanguages` and `customPrompt` replace the inherited field when set.
- **`override` starts fresh.** Configs above it are ignored for its entire subtree. The override config must therefore provide its own `targetLanguages`; it may also select its own model.
- **`untranslatableWords` and `excludeKeys` accumulate.** A subfolder adds its own brand names / excluded keys on top of the inherited ones (deduplicated union).
- **`model` stays inherited while merging.** A nested merge config cannot switch models; an override config can.
- **Android resource paths belong to the config that declares them.** Nested configs can add module roots. `androidSourceLanguage` is inherited while merging and can be changed in a nested config.
- **`exclude` is scoped.** An `exclude` entry only skips directories within the subtree of the config that declares it.

The project root is anchored at the **topmost config in the active merge chain**; an `override` config becomes the root when running inside its subtree. Create a nested config with `xlocal init` inside a subfolder — it detects the config above and creates a partial merge config that inherits from it.

## Conventions

xlocal builds its translation prompts from your catalog — the quality of what goes in decides the quality of what comes out. Also available in the terminal via `xlocal conventions`.

1. **Use explicit source text.** Xcode translations use English source strings. Android uses the text in `values/*.xml`, in the language configured by `androidSourceLanguage` (English by default); resource names are identifiers, not fallback text.
2. **Developer comments are written in English.** The comment of a key is sent to the model — it's your main way to give context: what the string is ("button label", "empty-state title") and where it appears.
3. **Existing translations are the reference.** Every existing translation of a key is included in the prompt, so new languages stay consistent with your established terminology.
4. **Brand and product names go into `untranslatableWords`.** xlocal instructs the model to keep them exactly as written and warns you when one was translated anyway.

## How translations work

For every missing (key, language) pair, xlocal builds a prompt containing the source text, the localization key, the developer comment, all existing translations of that key, your untranslatable words, and formality instructions. Plural strings use the target language’s CLDR categories, with all source plural forms supplied as context. The source format specifiers and argument positions are preserved; responses with incompatible format arguments are rejected before saving. Forms may omit the number when supported by the source variants and the target language’s grammar. Four translations run in parallel; you review a summary (including brand-word warnings) before anything is written.

Xcode catalogs are written in the exact format its String Catalog editor produces — byte-identical round trips, minimal diffs. Android translations are added to locale-specific XML files; existing nonempty translations and unrelated XML are preserved.

## Android support (experimental)

### Configuration

Run `xlocal init` at your Android project root (or use the locally built
binary shown below).
It detects standard `*/src/main/res` folders; review the selected roots and
add the languages you want to translate. If the project already has an
`xlocal-config.json`, add the Android fields to it instead:

```json
{
  "targetLanguages": ["en", "de", "fr"],
  "baseLanguages": ["en"],
  "androidResources": ["app/src/main/res"],
  "androidSourceLanguage": "en",
  "untranslatableWords": ["MyAppName"]
}
```

`androidSourceLanguage` describes the text in the unqualified `values`
directory. Set it explicitly if that text is not English. Resource paths
must stay inside the directory of the config that declares them.

All `values/*.xml` filenames are inspected, not just `strings.xml`.
Translations in `values-de`, `values-pt-rBR`, or BCP 47 folders such as
`values-b+zh+Hant` are matched by resource type and name, even when their
filenames differ. Missing entries go into the target locale folder using
the source filename, for example `values-fr/settings_strings.xml`.

### Test locally

Android support is available starting with v0.8.0. You can use the installed
`xlocal` command, or build from source to test locally. From the xlocal
repository root:

```sh
go build -o /private/tmp/xlocal-android-local/xlocal .
```

For an isolated trial, copy the bundled resource-only fixture to a temporary
folder. It contains missing translations and deliberately unsupported entries:

```sh
test_dir=$(mktemp -d /private/tmp/xlocal-android-test.XXXXXX)
cp -R testdata/androidproject/. "$test_dir/"
cd "$test_dir"
/private/tmp/xlocal-android-local/xlocal status
/private/tmp/xlocal-android-local/xlocal --dry-run
```

`status` (including `status --json`) and `--dry-run` do not call the API or
change resource files and do not require an API key. Warnings identify
resources the prototype cannot translate; they may currently repeat across
files. A complete status only covers supported, eligible resources.

To translate, run:

```sh
/private/tmp/xlocal-android-local/xlocal
```

Use the arrow keys and Enter to select a file, then choose **First 3 keys**
when that batch option is available. The command uses your configured Anthropic
API key, shows the translation results, and asks before saving. If needed,
store a key with `/private/tmp/xlocal-android-local/xlocal keys add`.
After saving, run `status` again and inspect the generated locale XML.

For your own app, use the same binary from the Android project root after
configuring it as shown above. For an isolated first test, copy the resource
folders into a separate directory, keeping the same relative paths, and create
the config there. A resource-only copy is enough for xlocal; a runnable app
is needed to check the result in Android Studio or on a device.

### Supported resources and limits

- Plain `<string>` and `<plurals>` resources are supported. Existing nonempty
  translations are kept; partial plurals receive only missing or empty forms.
  Source files, unrelated XML, comments and existing attributes are preserved.
- Java Formatter arguments such as `%1$s`, `%2$d` and `%1$.2f` are validated
  before saving. Foundation placeholders such as `%@` and `%lld` are rejected.
  For literal percent text, use `formatted="false"`.
- Android plurals use integer CLDR categories and retain the required `other`
  fallback. Languages outside the [supported table](internal/android/plurals.go)
  fail explicitly when plural rules are needed. Older Android versions may
  use older plural rules.
- `translatable="false"` and `excludeKeys` entries are excluded. Arrays,
  styled/nested text (including XLIFF elements), ICU/named placeholders,
  resource aliases, unfamiliar attributes and unsupported source formats
  are reported and skipped. XML directives, duplicate resource definitions
  and symlink resource paths are rejected.
- Resource roots are independent. Additional modules, product flavors and
  custom source sets must be listed explicitly in `androidResources`.
  Gradle overlays are not resolved. Folders with extra configuration
  qualifiers, such as `values-de-night`, are left untouched.
- xlocal reads existing XML resources; it does not extract hardcoded text
  from Kotlin, Java, Compose or layout files, or generate source keys.
- Each changed XML file is written atomically; saving several files is not
  a single transaction. Do not edit source strings during a translation run.
  macOS remains the supported CLI host.

The prototype has automated parsing, format validation and save-flow tests,
including optional checks with Android's resource compiler. A complete Gradle
app build and Android runtime/UI behavior have not yet been verified.

## Development

```sh
go test ./...
go build
```

To run Android resource compiler checks as well, point these variables to
an installed Android SDK (adjust the SDK versions to your installation):

```sh
export XLOCAL_AAPT2="$HOME/Library/Android/sdk/build-tools/36.0.0/aapt2"
export XLOCAL_ANDROID_JAR="$HOME/Library/Android/sdk/platforms/android-36/android.jar"
go test ./internal/android ./cmd
```

`XLOCAL_AAPT2` enables resource compilation checks. Setting
`XLOCAL_ANDROID_JAR` as well enables linking and comparisons of compiled
resource values. SDK-dependent checks are skipped when the tools are unavailable.

Releases are built with [GoReleaser](https://goreleaser.com) via GitHub Actions on tag push (`v*`) and published to `mobilepur/homebrew-tap`.

## License

[MIT](LICENSE)
