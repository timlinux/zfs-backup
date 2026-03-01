# Contributing

Thank you for your interest in contributing to Kartoza ZFS Backup Tool!

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork**:
   ```bash
   git clone https://github.com/YOUR_USERNAME/zfs-backup.git
   cd zfs-backup
   ```
3. **Enter the development shell**:
   ```bash
   nix develop
   ```
4. **Create a feature branch**:
   ```bash
   git checkout -b feature/my-new-feature
   ```

## Development Workflow

### Making Changes

1. Make your changes
2. Test locally:
   ```bash
   make build
   sudo ./zfs-backup
   ```
3. Ensure code is formatted:
   ```bash
   go fmt ./...
   ```
4. Run any tests:
   ```bash
   go test ./...
   ```

### Commit Messages

We follow conventional commit format:

```
type(scope): description

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Formatting, no code change
- `refactor`: Code change that neither fixes nor adds
- `test`: Adding tests
- `chore`: Maintenance

Examples:
```
feat(backup): add support for custom retention policies

fix(ui): correct progress bar calculation

docs(readme): update installation instructions
```

### Pull Requests

1. **Push your branch**:
   ```bash
   git push origin feature/my-new-feature
   ```
2. **Open a Pull Request** on GitHub
3. **Fill out the PR template** with:
   - Description of changes
   - Related issues
   - Testing performed
4. **Wait for review**

## Code Style

### Go Code

- Follow standard Go conventions
- Use `go fmt` for formatting
- Keep functions focused and small
- Add comments for exported functions
- Handle errors explicitly

### UI Components

- Use Kartoza brand colors (defined in `main.go`)
- Follow the DRY principle for header/footer
- Keep views responsive to terminal size

### Documentation

- Update docs when adding features
- Use clear, concise language
- Include examples where helpful

## Areas for Contribution

### Good First Issues

Look for issues labeled `good first issue`:

- Documentation improvements
- UI polish
- Error message improvements
- Test coverage

### Feature Ideas

- Configuration file support
- Custom retention policies
- Multiple backup destinations
- Notification system
- Backup scheduling

### Bug Fixes

- Check the issue tracker
- Reproduce the issue first
- Write a test if possible
- Fix the issue
- Verify the fix

## Testing

### Manual Testing

Since this tool interacts with ZFS, manual testing is important:

1. **Set up test pools** (use files as vdevs for safety):
   ```bash
   dd if=/dev/zero of=/tmp/test-pool bs=1M count=100
   sudo zpool create testpool /tmp/test-pool
   ```

2. **Test each operation**:
   - Incremental backup
   - Force backup
   - Prepare device
   - Unmount

3. **Test edge cases**:
   - Cancel and resume
   - Missing pools
   - Wrong password

### Automated Testing

```bash
# Run all tests
go test ./...

# With coverage
go test -cover ./...
```

## Questions?

- Open an issue for bugs or feature requests
- Start a discussion for questions
- Check existing issues first

## Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help others learn

---

Made with :heart: by [Kartoza](https://kartoza.com) | [Donate!](https://github.com/sponsors/kartoza) | [GitHub](https://github.com/kartoza/zfs-backup)
