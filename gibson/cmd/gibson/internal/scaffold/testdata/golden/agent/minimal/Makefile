BINARY := demo-agent

.PHONY: build test register run image

## build: compile the agent binary.
build:
	CGO_ENABLED=0 go build -o $(BINARY) .

## test: run unit tests.
test:
	go test ./...

## register: run the capability-grant enrollment handshake.
## Paste the single-use bootstrap token from the dashboard's Register wizard.
register:
	@echo "Run: gibson component register --token <bootstrap-token>"
	@echo "Get the bootstrap token from the Gibson dashboard's Register Agent wizard."

## run: run the compiled binary; reads the runtime credential from ~/.gibson/agent/.
run: build
	gibson component run

## image: build the container image.
image:
	docker build -t $(BINARY):0.1.0 .
