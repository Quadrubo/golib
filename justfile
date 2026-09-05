spec-protos := "grpcinterceptor/validate/testdata"

# Checks
check: fmt-check lint test

# Testing
test:
    go test ./...

# Linting
lint:
    golangci-lint run

# Formatting
fmt: fmt-go fmt-proto

fmt-go:
    golangci-lint fmt

fmt-proto:
    cd {{ spec-protos }} && buf format -w

fmt-check:
    golangci-lint fmt --diff
    cd {{ spec-protos }} && buf format --diff --exit-code

# Codegen
buf-gen:
    cd {{ spec-protos }} && buf generate

buf-update:
    cd {{ spec-protos }} && buf dep update

# Git
install-hooks:
    git config core.hooksPath .githooks
    @echo "hooks installed"
