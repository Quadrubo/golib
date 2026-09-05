# Checks
check: fmt-check lint test

# Testing
test:
    go test ./...

# Linting
lint:
    golangci-lint run

# Formatting
fmt:
    golangci-lint fmt

fmt-check:
    golangci-lint fmt --diff

# Git
install-hooks:
    git config core.hooksPath .githooks
    @echo "hooks installed"
