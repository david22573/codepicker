BINARY_NAME=codepicker

# Versioning info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags to inject variables into cmd package
LDFLAGS = -ldflags "-X github.com/david22573/codepicker/cmd.Version=$(VERSION) \
    -X github.com/david22573/codepicker/cmd.GitCommit=$(COMMIT) \
    -X github.com/david22573/codepicker/cmd.BuildDate=$(DATE)"

all: build

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) main.go

test:
	go test ./...

vet:
	go vet ./...

smoke: build
	./$(BINARY_NAME) --help
	./$(BINARY_NAME) init --help
	./$(BINARY_NAME) pack --help
	./$(BINARY_NAME) run --help
	./$(BINARY_NAME) plans --help
	./$(BINARY_NAME) apply --help
	./$(BINARY_NAME) undo --help

prove: test vet smoke

install:
	go install $(LDFLAGS)

clean:
	go clean
	rm -f $(BINARY_NAME)
	rm -f codepicker_context.txt *_context.md
	rm -rf codepicker_out

