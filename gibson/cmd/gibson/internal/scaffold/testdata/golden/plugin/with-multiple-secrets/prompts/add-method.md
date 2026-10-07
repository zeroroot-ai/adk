# Adding a method to byte-identity-multi

A plugin method is a single RPC: a declared name plus a typed Go
request/response pair. This is **Go-first** (ADR-0065 R4) — there is no
`.proto` and no manifest file (ADR-0097). Adding a method is a two-step change
touching `handler.go` and the cassette in `testdata/`.

## Step 1 — add the typed handler in handler.go

```go
type SendMessageRequest struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

type SendMessageResponse struct {
	MessageID    string `json:"message_id"`
	PostedAtUnix int64  `json:"posted_at_unix"`
}

func sendMessage(ctx context.Context, req SendMessageRequest) (SendMessageResponse, error) {
	// If you need a credential, resolve it from the context the SDK hands you
	// (plugin.ResolveSecret). A tenant admin grants the plugin the secret.
	// Never read from env vars, and never return the secret value in an error.
	msgID, ts, err := postToSlack(ctx, req.Channel, req.Text)
	if err != nil {
		return SendMessageResponse{}, fmt.Errorf("send_message: post: %w", err)
	}
	return SendMessageResponse{MessageID: msgID, PostedAtUnix: ts}, nil
}
```

Register it in `main()`:

```go
plugin.Serve(
	ctx,
	plugin.WithName(pluginName),
	plugin.WithVersion(pluginVersion),
	plugin.WithHandler("Echo", "Echo returns the request message unchanged.", echo),
	plugin.WithHandler("SendMessage", "Post a message to a channel.", sendMessage), // new
)
```

The SDK derives the JSON-Schema contract for `SendMessage` from the two structs
at registration. A field the schema deriver cannot express (an `any`/interface)
is a startup error. The description is required: an agent reads it to choose
between tools.

## Step 2 — record a cassette and add a test

Add `testdata/send_message.json`:

```json
{
  "request":  { "channel": "#alerts", "text": "hello" },
  "response": { "message_id": "M-1", "posted_at_unix": 0 }
}
```

Add a sub-test in `handler_test.go` that loads it and calls `sendMessage`. Keep
it hermetic — no daemon, no network.

## Validate

```sh
go test ./...                           # runs the cassette tests
gibson component validate --kind plugin # checks that the Go source parses
```

`plugin.Serve` refuses to start on a handler with no description or with a
type the schema deriver cannot express.

## Don't

- Don't add `request_proto` / `response_proto` or a `.proto` — the contract is
  derived from Go.
- Don't add a manifest file. The plugin declares itself in code.
- Don't return a secret value in `error.Error()`.
