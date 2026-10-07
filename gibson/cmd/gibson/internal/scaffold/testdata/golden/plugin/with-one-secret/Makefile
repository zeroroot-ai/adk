BINARY := byte-identity-secret

.PHONY: build test run image

## build: compile the plugin binary (CGO_ENABLED=0).
build:
	CGO_ENABLED=0 go build -o $(BINARY) .

## test: run the hermetic handler tests (cassette replay, no daemon/network).
test:
	go test ./...

## run: start the plugin via the CLI's process supervisor.
## Enrolment happens on the first start, from two environment variables
## (sdk#128): GIBSON_URL and GIBSON_BOOTSTRAP_TOKEN. `gibson component register`
## existed only to read component.yaml and was deleted with it (adk#90).
run: build
	gibson component run --kind plugin

## image: build the container image.
image:
	docker build -t $(BINARY):1.2.3 .
