# Meeting agenda and minutes agent — developer tasks.
#
# The deterministic suite needs no API key: the ADK layer runs on an injected
# fake model. Only `run` and `serve` call Gemini, and both need GOOGLE_API_KEY.

BINARY := meeting-agenda-minutes-agent
IMAGE  := $(BINARY):local
OUT    := ./out

.PHONY: help check-fmt fmt vet build test test-race run run-headless docker-build docker-smoke clean

help: ## List the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

check-fmt: ## Fail when a Go file is unformatted
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "unformatted:"; echo "$$files"; exit 1; fi

fmt: ## Format every Go file in place
	gofmt -w .

vet: ## Run go vet over every package
	go vet ./...

build: ## Build the binary into bin/
	go build -trimpath -o bin/$(BINARY) .

test: ## Run the deterministic suite (no API key)
	go test ./... -count=1

test-race: ## Run the deterministic suite with the race detector
	go test ./... -count=1 -race

run: ## Serve the dry-run UI on loopback (needs GOOGLE_API_KEY)
	go run . serve -addr 127.0.0.1:8080 -out $(OUT)

run-headless: ## Draft one proposal from INPUT=path/to/meeting.json (needs GOOGLE_API_KEY)
	@test -n "$(INPUT)" || (echo "usage: make run-headless INPUT=path/to/meeting.json" && exit 2)
	go run . run -input $(INPUT) -out $(OUT)

docker-build: ## Build the container image
	docker build -t $(IMAGE) .

docker-smoke: ## Run the keyless smoke checks inside the image
	docker run --rm $(IMAGE) -h
	docker run --rm $(IMAGE) run -dry-run=false || test $$? -eq 2

clean: ## Remove build output and dry-run artifacts
	rm -rf bin $(OUT)
