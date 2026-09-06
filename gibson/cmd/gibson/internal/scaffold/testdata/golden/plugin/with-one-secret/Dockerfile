# Stage 1: build — compiles the plugin binary with CGO disabled.
FROM golang:1.26.4-alpine AS builder

WORKDIR /build
# go.sum may not exist yet on a freshly scaffolded plugin; the glob tolerates
# its absence so the image builds before the first `go mod tidy`.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/byte-identity-secret .

# Stage 2: runtime — minimal image for the pod runtime mode.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

COPY --from=builder /out/byte-identity-secret /byte-identity-secret
COPY plugin.yaml /etc/gibson/plugin.yaml

EXPOSE 8080

ENTRYPOINT ["/byte-identity-secret"]
