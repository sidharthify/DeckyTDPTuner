GOFLAGS ?= -trimpath

.PHONY: all
all: out/tdptuner

out/tdptuner: $(shell find . -name "*.go") go.mod
	mkdir -p out
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "-s -w" -o out/tdptuner ./cmd/tdptuner

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	rm -rf out
