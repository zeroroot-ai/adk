# Stage 1: build — compiles the agent binary with CGO disabled.
FROM golang:1.26.8-alpine AS builder
# GOTOOLCHAIN=local: the builder must carry the Go that go.mod names, so a
# mismatch fails the build instead of downloading a toolchain.
ARG GOTOOLCHAIN=local
ENV GOTOOLCHAIN=${GOTOOLCHAIN}

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/demo-agent .

# Stage 2: runtime — distroless, non-root.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

COPY --from=builder /out/demo-agent /demo-agent
COPY component.yaml /component.yaml

EXPOSE 50051

ENTRYPOINT ["/demo-agent"]
