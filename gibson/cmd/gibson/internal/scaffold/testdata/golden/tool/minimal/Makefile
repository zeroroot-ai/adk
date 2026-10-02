BINARY := demo-tool

.PHONY: proto build test run image

## proto: regenerate Go bindings from api/proto/gibson/tools/demotool/v1/demotool.proto.
## Requires buf (https://buf.build/docs/installation), protoc-gen-go, and
## protoc-gen-go-grpc on PATH.
proto:
	buf generate

## build: compile the tool binary.
build:
	CGO_ENABLED=0 go build -o $(BINARY) .

## test: run unit tests.
test:
	go test ./...

## run: run the compiled binary; reads the runtime credential from ~/.gibson/tool/.
## Enrolment happens on the first start, from two environment variables
## (sdk#128): GIBSON_URL and GIBSON_BOOTSTRAP_TOKEN. `gibson component register`
## existed only to read component.yaml and was deleted with it (adk#90).
run: build
	gibson component run --kind tool

## image: build the container image.
image:
	docker build -t $(BINARY):0.1.0 .
