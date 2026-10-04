# Contributing

Thanks for helping with WeeJ. Bug reports, ideas, translations and code are all welcome.

## Issues

- **Bugs and ideas**: [open an issue](https://github.com/zolferfigueiredo/weej/issues/new/choose). Search the open ones first.
- **Security problems**: don't open an issue. Follow the [security policy](SECURITY.md).

## Pull requests

For anything bigger than a small fix, open an issue first so we can agree on the idea before you build it.

1. Fork the repository and branch from `main`.
2. Make your change. `run.bat` builds WeeJ and runs it in the terminal, and `go test ./...` runs the tests. You need Windows 10 or 11 and Go 1.27; a deej board helps.
3. If you changed the app (anything in `main.go`, `internal`, `tools`, `go.mod`, `go.sum` or `winres`), raise `AppVersion` in `internal/core/version.go`, for example 1.2.3 to 1.2.4.
4. Open a pull request against `main`.

CI builds and tests it on Linux. It also runs shellcheck on the git hook and checks that the version went up.

## Translations

Each language has its own file in [`internal/lang/catalogs`](../internal/lang/catalogs). To fix a translation, edit that file. Don't use em or en dashes there, use a comma, colon, full stop or parentheses instead.

## License

By contributing, you agree that your work is licensed under the [MIT License](../LICENSE).
