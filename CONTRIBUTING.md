# Contributing to WP Maintenance Automation Go

Thank you for your interest in contributing to WP Maintenance Automation Go! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Commit your changes (`git commit -m 'feat: add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

## Development Workflow

### Prerequisites

Make sure you have the following installed:
- Go 1.21 or higher
- Docker and Docker Compose (for development/testing)
- Git
- SSH client access to test servers
- restic (for backup testing)
- MySQL/MariaDB client tools

### Running Tests

Run all tests:
```bash
make test
```

Run tests with coverage:
```bash
make test-cover
```

Run specific test suites:
```bash
go test ./internal/backup/...
go test ./internal/upgrade/...
go test ./internal/restore/...
go test ./internal/healthcheck/...
go test ./internal/apiserver/...
```

Run linter:
```bash
make lint
```

Format code:
```bash
make fmt
```

### Code Quality

Before submitting a pull request, ensure your code meets these standards:

1. **Go Code**: Follow Go best practices and idioms
2. **Formatting**: Use `go fmt` or `make fmt` for formatting
3. **Testing**: Add tests for new functionality
4. **Documentation**: Add godoc comments for exported functions and types
5. **Error Handling**: Include proper error handling
6. **Security**: Follow security best practices

### Go Best Practices

- Use `gofmt` for formatting
- Use `go vet` for static analysis
- Use `golint` for linting (if available)
- Include proper error handling
- Use context for cancellation
- Follow Go naming conventions
- Use interfaces for abstraction
- Include proper documentation

### Environment Variables

- Document all environment variables in `.env.example`
- Use descriptive variable names
- Provide sensible defaults where possible
- Use environment-specific configurations

### Documentation

- Keep documentation up to date
- Update README.md for new features
- Add inline comments for complex code
- Document API endpoints and usage

### Testing

- Add tests for new functionality
- Ensure tests pass before submitting PRs
- Test edge cases and error scenarios
- Use Go's testing framework
- Include table-driven tests for functions with multiple cases
- Use mock objects for external dependencies

## Pull Request Guidelines

### Title Format

Use conventional commit format:

```
<type>(<scope>): <description>

type: feat, fix, docs, style, refactor, test, chore
scope: backup, upgrade, restore, healthcheck, api, etc.
```

Examples:
- `feat(backup): add compression option`
- `fix(upgrade): resolve healthcheck timeout`
- `docs(readme): update setup instructions`
- `refactor(api): improve error handling`

### PR Description

Include:
- Summary of changes
- Type of change (bug fix, new feature, etc.)
- Related issues
- Instructions for testing
- Screenshots (for UI changes)

### Code Review Process

1. Wait for code review
2. Address reviewer comments
3. Update the PR description if needed
4. Ensure all tests pass
5. Update documentation if needed

## Specific Guidelines

### API Development

- Follow RESTful API design principles
- Use proper HTTP status codes
- Include error messages in responses
- Document API endpoints
- Use OpenAPI/Swagger for API documentation

### Backup/Restore

- Ensure data integrity
- Include proper error handling
- Support concurrent operations
- Include logging for debugging
- Test with realistic data

### Upgrade Functionality

- Test with multiple WordPress versions
- Include rollback capability
- Run health checks after upgrades
- Log all operations
- Handle edge cases

### Security

- Never commit secrets to the repository
- Use environment variables for sensitive data
- Validate all inputs
- Use proper authentication and authorization
- Follow OWASP guidelines

## Questions?

If you have questions about contributing:

1. Check the existing documentation in the repository
2. Open an issue for clarification
3. Contact project maintainers

## License

By contributing, you agree that your contributions will be licensed under the MIT License. See [LICENSE](LICENSE) for details.

## Recognition

Contributors will be recognized in:
- The README.md acknowledgments section
- Project commit history

## Getting Help

- Check the existing documentation
- Search for similar issues
- Open an issue for questions
- Join our community discussions

Thank you for helping make WP Maintenance Automation Go better!
