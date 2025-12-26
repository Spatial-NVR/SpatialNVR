# Contributing to NVR System

Thank you for contributing to the NVR system. This document provides guidelines for contributions.

## Documentation Style Guide

### General Rules

1. **No emojis** - Do not use emojis in any documentation, code comments, commit messages, or configuration files.

2. **Clear and concise** - Write documentation that is easy to scan and understand quickly.

3. **Use tables** - For lists of options, configurations, or comparisons, prefer tables over prose.

4. **Include examples** - Every configuration option or command should have an example.

5. **Test commands** - All shell commands in documentation must be tested before committing.

### Markdown Formatting

- Use ATX-style headers (`#`, `##`, `###`)
- Use fenced code blocks with language identifiers
- Use tables for structured data
- One sentence per line in source (for better diffs)
- Include a table of contents for documents longer than 3 sections

### Code Examples

Good:
```yaml
# data/config.yaml
detection:
  backend: cuda
  device_id: 0
```

Bad:
```
detection:
  backend: cuda  # Use CUDA backend!
```

### Commit Messages

Format:
```
<type>: <short description>

<optional longer description>

<optional footer>
```

Types:
- `feat` - New feature
- `fix` - Bug fix
- `docs` - Documentation only
- `refactor` - Code refactoring
- `test` - Adding tests
- `chore` - Maintenance tasks

Example:
```
feat: add GPU acceleration for detection service

Added CUDA backend support for detection service. Users can now
configure GPU acceleration by setting detection.backend=cuda.

See docs/SCALING.md for configuration details.
```

Do not use emojis in commit messages.

## Code Style

### Go

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable names
- Comment exported functions and types
- Handle all errors explicitly

### TypeScript/React

- Use functional components with hooks
- Use TypeScript strict mode
- Prefer named exports over default exports
- Use `camelCase` for variables, `PascalCase` for components

### Python

- Follow PEP 8
- Use type hints
- Use docstrings for public functions

## Testing

- Write tests for new features
- Maintain existing test coverage
- Run tests before submitting PRs

```bash
# Go tests
go test ./...

# Frontend tests
cd web-ui && npm test

# Python tests
cd services/ai-detection && pytest
```

## Pull Request Process

1. Create a feature branch from `main`
2. Make your changes with appropriate tests
3. Update documentation if needed
4. Run all tests locally
5. Submit PR with clear description
6. Address review feedback

## Questions

For questions about contributing, open a GitHub issue or discussion.
