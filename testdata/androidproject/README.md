# Android local prototype fixture

This is a resource-only Android fixture, not a runnable Gradle application.
It deliberately has missing translations and unsupported resources so the
prototype's status, warnings and preservation behavior are visible.

From this directory, use the locally built binary:

```sh
/private/tmp/xlocal-android-local/xlocal status
/private/tmp/xlocal-android-local/xlocal --dry-run
/private/tmp/xlocal-android-local/xlocal
```

The last command uses your configured Anthropic key, shows the translations
and asks before saving. For a real app, run `xlocal init` at its root, review
`androidResources`, and set `androidSourceLanguage` to the language in `values`.
The default is English. No Kotlin or layout text is extracted automatically.

Arrays, styled text, aliases, and folders with additional qualifiers such as
`values-de-night` are outside this prototype. Existing supported translations
are kept; partial plurals receive only missing forms. Resource roots are
independent; Gradle overlays and runtime layout have not been tested.
