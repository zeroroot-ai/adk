package main

import (
	"cmp"
	"context"
	"log/slog"
	"os"

	"github.com/zeroroot-ai/sdk/plugin"
)

// EchoRequest is the typed request for the Echo method.
//
// Go-first authoring (ADR-0065 R4): the SDK derives this method's JSON-Schema
// input contract from this struct at registration. There is no .proto and no
// generated code — the Go type IS the contract. Replace these fields with the
// real request shape for your integration.
type EchoRequest struct {
	Message string `json:"message"`
}

// EchoResponse is the typed response for the Echo method. Its JSON-Schema
// output contract is likewise derived from this struct.
type EchoResponse struct {
	Message string `json:"message"`
}

// echo returns the request message unchanged. This is the one place your
// integration logic lives; the SDK handles decode, dispatch, and encode.
//
// Never put secret values in the returned error.
func echo(_ context.Context, req EchoRequest) (EchoResponse, error) {
	return EchoResponse{Message: req.Message}, nil
}

func main() {
	err := plugin.Serve(
		context.Background(),
		plugin.WithManifest(cmp.Or(os.Getenv("GIBSON_PLUGIN_MANIFEST"), "./plugin.yaml")),
		plugin.WithHandler("Echo", echo),
	)
	if err != nil {
		slog.Error("plugin exited with error", "err", err)
		os.Exit(1)
	}
}
