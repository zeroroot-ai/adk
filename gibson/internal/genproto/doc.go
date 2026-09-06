// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

// Package genproto holds daemon gRPC clients the OSS gibson CLI needs that
// are NOT published in the OSS SDK (github.com/zeroroot-ai/sdk/api/gen).
//
// By ADR-0058 the gibson.tenant.v1 services live in the closed gibson
// platform protos, not the OSS SDK, so the CLI cannot import them from the
// SDK the way it imports gibson.agentidentity.v1. ConnectorService
// (ADR-0014 Slice 4) is one such service. Its Go client is generated HERE,
// into the ADK module, from a client-only copy of connector.proto staged
// under gibson/tenant/v1/.
//
// The staged copy strips the server-only (gibson.auth.v1.authz) method
// options; the wire contract (the gibson.tenant.v1 package name, the
// ConnectorService service name, the RPC method names and the message field
// numbers) is byte-for-byte the daemon's, so the generated client talks to
// the real daemon unchanged.
//
// To regenerate after the platform proto changes, restage the proto and run:
//
//	protoc \
//	  --proto_path=<stage-root> \
//	  --go_out=. --go_opt=module=github.com/zeroroot-ai/adk/gibson \
//	  --go-grpc_out=. --go-grpc_opt=module=github.com/zeroroot-ai/adk/gibson \
//	  gibson/tenant/v1/connector.proto
//
// from the module root, with protoc-gen-go built from the pinned
// google.golang.org/protobuf and protoc-gen-go-grpc on PATH.
package genproto
