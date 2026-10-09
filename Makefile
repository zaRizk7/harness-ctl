GO := go
export GOTOOLCHAIN := local

.PHONY: build check coverage run tidy hooks
build:
	mkdir -p bin
	$(GO) build -trimpath -o bin/harness-ctl ./cmd/harness-ctl

check:
	./scripts/check-go.sh

coverage:
	./scripts/coverage-go.sh

run: build
	./bin/harness-ctl

tidy:
	$(GO) mod tidy

hooks:
	pre-commit install --install-hooks --hook-type pre-commit --hook-type pre-merge-commit --hook-type commit-msg --hook-type pre-push
