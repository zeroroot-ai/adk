# Changelog

## [0.111.0](https://github.com/zeroroot-ai/adk/compare/v0.110.1...v0.111.0) (2026-10-04)


### Features

* **mission:** the embedded CUE schema knows a mission declares its secrets ([#97](https://github.com/zeroroot-ai/adk/issues/97)) ([7763261](https://github.com/zeroroot-ai/adk/commit/77632618a8385837282145218c7b23a548c3a3aa))

## [0.110.1](https://github.com/zeroroot-ai/adk/compare/v0.110.0...v0.110.1) (2026-10-02)


### Bug Fixes

* **ci:** adk required three checks while fourteen ran ([#93](https://github.com/zeroroot-ai/adk/issues/93)) ([69fc4bb](https://github.com/zeroroot-ai/adk/commit/69fc4bbe34d420a311e72648f4d558ebc7d6c841))
* **ci:** move the reusable-go-ci pin past the govulncheck panic ([#87](https://github.com/zeroroot-ai/adk/issues/87)) ([171c4c1](https://github.com/zeroroot-ai/adk/commit/171c4c13ef0961fed38df28b6352c678a5cff938))
* **cli:** verify the OIDC issuer, and drop two config keys nothing reads ([#85](https://github.com/zeroroot-ai/adk/issues/85)) ([8d1ca3a](https://github.com/zeroroot-ai/adk/commit/8d1ca3a0f67be9645a592d3a90719fca95790e86))
* **component:** drop a spec key no build step reads, and make the deadcode pin real ([#89](https://github.com/zeroroot-ai/adk/issues/89)) ([1f948cf](https://github.com/zeroroot-ai/adk/commit/1f948cf7185a6d91d13593a340fdcbf723e0562c))

## [0.110.0](https://github.com/zeroroot-ai/adk/compare/v0.109.5...v0.110.0) (2026-10-02)


### Features

* **go:** move the toolchain floor to 1.27.1 ([#76](https://github.com/zeroroot-ai/adk/issues/76)) ([423aae9](https://github.com/zeroroot-ai/adk/commit/423aae98095b04ebb189e013eac98a13ccef9f3f))
* **secret:** gibson secret, the whole SecretsService surface ([#81](https://github.com/zeroroot-ai/adk/issues/81)) ([09257e1](https://github.com/zeroroot-ai/adk/commit/09257e1964848a8aed99f4472787d6bdb6f62d6a))


### Bug Fixes

* **cli:** the CLI reads the proxy environment, and keeps Go's transport defaults ([#79](https://github.com/zeroroot-ai/adk/issues/79)) ([c8d3388](https://github.com/zeroroot-ai/adk/commit/c8d338834f68e20bb609488ac7ff3e135550754c))
* **deadcode:** the gate reported ok while analysing nothing ([#82](https://github.com/zeroroot-ai/adk/issues/82)) ([11b98a8](https://github.com/zeroroot-ai/adk/commit/11b98a88488fc91ae5890b4bc156d45d6d45ac1a))
* **mission:** scaffold a target, so every template submits as written ([#70](https://github.com/zeroroot-ai/adk/issues/70)) ([7385760](https://github.com/zeroroot-ai/adk/commit/7385760bce19f81a180aa7fbb9ecefe7969cc0bb))

## [0.109.5](https://github.com/zeroroot-ai/adk/compare/v0.109.4...v0.109.5) (2026-10-01)


### Bug Fixes

* **ci:** link-check checks only the Markdown a PR touched (.github v0.7.2) ([#57](https://github.com/zeroroot-ai/adk/issues/57)) ([8e5d9f3](https://github.com/zeroroot-ai/adk/commit/8e5d9f3a75452564e363c2e1fc67138ac9682892))
* **ci:** pin every zeroroot-ai/.github reference to v0.4.1 ([#34](https://github.com/zeroroot-ai/adk/issues/34)) ([a24f5f0](https://github.com/zeroroot-ai/adk/commit/a24f5f07ba15c156a8f6fc8ef54f8238007abb98))
* **ci:** pin every zeroroot-ai/.github reference to v0.5.1 ([#35](https://github.com/zeroroot-ai/adk/issues/35)) ([8505557](https://github.com/zeroroot-ai/adk/commit/8505557aaae47ad4cc921b800f9bb258f19e6c1f))
* **ci:** pin the org tree guards to a commit SHA ([#30](https://github.com/zeroroot-ai/adk/issues/30)) ([d4aaf4d](https://github.com/zeroroot-ai/adk/commit/d4aaf4d185d1bcad02918228f63cc2e774a8eefb))
* **cue:** regenerate the embedded mission CUE from the sdk proto ([#37](https://github.com/zeroroot-ai/adk/issues/37)) ([eed8ac0](https://github.com/zeroroot-ai/adk/commit/eed8ac0221cdf2efaab4e8c85a91fc9b43a86cfc))
* **gibson-cli:** default to api.zeroroot.ai and survive the login rate limit ([#59](https://github.com/zeroroot-ai/adk/issues/59)) ([4bea824](https://github.com/zeroroot-ai/adk/commit/4bea82423b64226737f3e9f58780bf9849ff560c))
* **gibson-cli:** fixes from the CLI test run ([#64](https://github.com/zeroroot-ai/adk/issues/64)) ([b04a83c](https://github.com/zeroroot-ai/adk/commit/b04a83c40f7d742f8855a7b6ffc7ac064a79bf6f))
* **gibson-cli:** never send x-gibson-tenant for a person's session ([#51](https://github.com/zeroroot-ai/adk/issues/51)) ([057bc57](https://github.com/zeroroot-ai/adk/commit/057bc5744fafc6c964abaaa44ca8e56f17fba973))
* **gibson-cli:** satisfy new-code lint on the tenant-header removal ([#52](https://github.com/zeroroot-ai/adk/issues/52)) ([27b31b7](https://github.com/zeroroot-ai/adk/commit/27b31b7d99ad9b1a23f7277ac2ba74fd5195ff9e))


### Performance Improvements

* **ci:** cache the golangci-lint binary instead of compiling it on every run ([#53](https://github.com/zeroroot-ai/adk/issues/53)) ([7f5f038](https://github.com/zeroroot-ai/adk/commit/7f5f03893b9a2e587690ad5c7c589ed96791ddf6))

## [0.109.4](https://github.com/zeroroot-ai/adk/compare/v0.109.3...v0.109.4) (2026-09-09)


### Bug Fixes

* **ci:** grant pull-requests: read to the doc-coverage caller ([#21](https://github.com/zeroroot-ai/adk/issues/21)) ([d30ab09](https://github.com/zeroroot-ai/adk/commit/d30ab097d7891d05bc473b8035a7356c0d111b74))

## Changelog

This repository restarted from a fresh baseline on 2026-09-06. Release notes before that date are archived offline and do not resolve on GitHub. release-please adds each release below this line.
