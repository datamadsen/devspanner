# Contributing to devspanner

Thanks for taking the time to contribute! devspanner is a small, focused tool, so
the bar is simple: keep it small, keep it focused, keep it honest about what it does.

## Getting started

```bash
git clone https://github.com/datamadsen/devspanner
cd devspanner
go build ./...        # builds the binary
go test ./...         # runs the tests
```

To try your build against a real project, point it at a repo that has a
`.devspanner/config.yaml` (see [`examples/config.yaml`](examples/config.yaml)).

## Before you open a pull request

Run the same checks CI runs:

```bash
make check            # fmt-check + vet + lint + test, or run them individually:
gofmt -l .            # must print nothing
go vet ./...
golangci-lint run     # https://golangci-lint.run/welcome/install/
go test -race ./...
```

- **Format** with `gofmt` (the `make fmt` target does it for you).
- **Add tests** for new logic where it's practical — especially pure helpers and
  config validation.
- **Keep commits focused** and write a clear message explaining the *why*.

## Scope & design

devspanner manages the local dev loop; it does not replace your build/deploy
scripts — it runs them. Behaviour is **derived from config fields, not a `type`**
(a `container:` makes a service docker-style, a `port:` makes it process-style).
New features should fit that model. If you're unsure whether something is in scope,
open an issue to discuss before writing code.

Note that the tool is **Linux-first**: ownership and stale-build detection read
`/proc`, and process management uses Unix process groups. macOS is supported with
graceful degradation; Windows is not a target.

## Reporting bugs & requesting features

Use the issue templates. For bugs, include your `.devspanner/config.yaml` (redacted
as needed), your OS, and the output of `devspanner -v`.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating, you agree to uphold it.

## License

By contributing, you agree that your contributions will be licensed under the
project's [MIT License](LICENSE).
