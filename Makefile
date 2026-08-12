.PHONY: check fmt vet lint test

check: fmt vet lint test

fmt:
	@echo "==> gofmt"
	@bash -c 'files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "$$files"; echo "gofmt: files need formatting"; exit 1; fi'

vet:
	@echo "==> go vet"
	go vet ./...

lint:
	@echo "==> golangci-lint"
	golangci-lint run

test:
	@echo "==> go test"
	# -count=1 disables the test result cache. This matters specifically for
	# ./architecture/...: its tests load packages from disk at run time via
	# golang.org/x/tools/go/packages (a dynamic filesystem read Go's build
	# cache does not track as a test input), so a cached "ok" could mask a
	# real, newly introduced architecture violation elsewhere in the repo.
	go test -race -cover -count=1 ./...
