# Contributing to OpenAIDE

Thanks for your interest! Here's how to help.

## Quick Start

```bash
git clone https://github.com/lzy1102/openaide.git
cd openaide/backend
make build
./bin/openaide
```

## Areas to Contribute

| Area | Skill Level | Impact |
|------|-------------|--------|
| Add LSP server commands | Beginner | Medium |
| Improve i18n translations | Beginner | High |
| Add tool handlers | Intermediate | High |
| Optimize prompts | Intermediate | Medium |
| Write tests | Any | High |
| Documentation | Any | Medium |

## Development Flow

1. Fork the repo
2. Create a branch: `feature/your-feature`
3. Make changes, add tests
4. `make test` to verify
5. Push and open a PR

## Architecture

See [OPENAIDE.md](./OPENAIDE.md) for the full architecture document loaded by the AI agent.
See [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) for the technical design.

## Code Style

- Go: standard `gofmt`, error handling required
- Prompts: in `internal/kernel/kernel_prompt.go` or `~/.openaide/data/prompts/`
- See [OPENAIDE.md](./OPENAIDE.md) Output Quality Rules for commit message style

## Questions?

Open an issue or start a [GitHub Discussion](https://github.com/lzy1102/openaide/discussions).
