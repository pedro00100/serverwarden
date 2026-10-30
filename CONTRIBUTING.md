# Contributing

Thanks for helping. Issues and pull requests are welcome; for anything larger than a
fix, open an issue first so we can agree on the approach.

**Security problems are not reported here:** see [SECURITY.md](SECURITY.md).

## Setup

You only need Podman or Docker: Go and Node run in throwaway containers.

```sh
make dev          # UI with sample data at http://localhost:5173
make test vet     # Go tests and static analysis
make smoke        # the panel binary against a real sshd
make ui-check     # with make dev running: every page in headless Chromium
make install-e2e  # the real image, a Debian server and Chromium, end to end
```

## Guidelines

- **Keep it small and dependency-light.** Prefer the Go standard library; add a
  dependency only when it is well maintained and clearly better than writing it.
- **Security first.** Never store or return a secret in plain text (seal it with
  `internal/adapters/secrets`), never log one, and keep the panel unable to do more on
  a server than that server's `agent.json` allows.
- **Architecture.** Business rules live in `internal/domain` and `internal/app`;
  anything that touches the outside world is an adapter behind a port.
- **Code and comments in English, few comments.** UI text goes through i18n: add the
  English key in `web/src/lib/i18n/en.js` (and Spanish in `es.js` if you can).
- **Tests.** New behaviour comes with a test; UI changes must pass `make ui-check`.
- **Never commit real configuration**: addresses, hostnames, keys or settings from your
  own network. Use the fictional `examples/homelab` network instead.

By contributing you agree that your work is licensed under the [AGPL-3.0](LICENSE).
