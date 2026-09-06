# Changelog

## [0.109.0](https://github.com/zeroroot-ai/adk/compare/v0.108.0...v0.109.0) (2026-08-29)


### Features

* **mission:** gibson mission submit --detach, and print node error text ([#243](https://github.com/zeroroot-ai/adk/issues/243)) ([54eff6a](https://github.com/zeroroot-ai/adk/commit/54eff6a96a84cce6b5dbc2ac243e9c0c84f7d48e))

## [0.108.0](https://github.com/zeroroot-ai/adk/compare/v0.107.1...v0.108.0) (2026-08-29)


### Features

* **cli:** gibson connector — catalog/enable/list/disable (ADR-0014) ([#226](https://github.com/zeroroot-ai/adk/issues/226)) ([36648e5](https://github.com/zeroroot-ai/adk/commit/36648e5b66f12f18d263bd919ed151c8ea6762c6))
* **component:** go-first plugin scaffold + connector kind ([#1520](https://github.com/zeroroot-ai/adk/issues/1520)) ([#229](https://github.com/zeroroot-ai/adk/issues/229)) ([f750794](https://github.com/zeroroot-ai/adk/commit/f750794238291a9a6e2c18fcf3ac71687b8be864))


### Bug Fixes

* **cli:** make gibson login honour --ca-cert / GIBSON_CA_CERT ([#224](https://github.com/zeroroot-ai/adk/issues/224)) ([a620aee](https://github.com/zeroroot-ai/adk/commit/a620aeebcbb15f5aa6be1cac0d99ca457edc734d))
* **scaffold:** plugin scaffold loads manifest from GIBSON_PLUGIN_MANIFEST (ADR-0065) ([#230](https://github.com/zeroroot-ai/adk/issues/230)) ([8ecfbe1](https://github.com/zeroroot-ai/adk/commit/8ecfbe12bc8171af207fe6ce8a2265131339f697))

## [0.107.1](https://github.com/zeroroot-ai/adk/compare/v0.107.0...v0.107.1) (2026-08-16)


### Bug Fixes

* **ci:** run release-please as the zeroday-sdk-fanout App ([#210](https://github.com/zeroroot-ai/adk/issues/210)) ([e7a7132](https://github.com/zeroroot-ai/adk/commit/e7a713239e7897eb032545731d5e32fc6e84bf7b))

## [0.107.0](https://github.com/zeroroot-ai/adk/compare/v0.106.0...v0.107.0) (2026-08-15)


### Features

* **ci:** notify docs-site to refresh cli-spec on release ([#200](https://github.com/zeroroot-ai/adk/issues/200)) ([082a172](https://github.com/zeroroot-ai/adk/commit/082a172926c0c61610a1aa737bf180cda62fa82e))
* **cli:** --ca-cert so a private-CA install works ([#180](https://github.com/zeroroot-ai/adk/issues/180)) ([4ade28c](https://github.com/zeroroot-ai/adk/commit/4ade28c787fab57b4ed83f827c764fc9c4d3e486)), closes [#178](https://github.com/zeroroot-ai/adk/issues/178)
* **docs:** emit the gibson command tree as machine-readable JSON ([#184](https://github.com/zeroroot-ai/adk/issues/184)) ([f872d70](https://github.com/zeroroot-ai/adk/commit/f872d7094277c54348bf49b2811b98802291738f))


### Bug Fixes

* **ci:** move CodeQL off merge_group — SARIF upload always fails there ([#207](https://github.com/zeroroot-ai/adk/issues/207)) ([55be3ca](https://github.com/zeroroot-ai/adk/commit/55be3cab8d3739fcd5584567404a3aac03d3543e))
* **ci:** pin actions to commit SHAs, tighten permissions, patch CVEs ([#202](https://github.com/zeroroot-ai/adk/issues/202)) ([2c26a28](https://github.com/zeroroot-ai/adk/commit/2c26a28262cb9091e3b45248cbb751f2d6eaf9ec))
* **ci:** prime buf via go-tool-buf, not npm ci (check-cue-fresh) ([#186](https://github.com/zeroroot-ai/adk/issues/186)) ([c10f7be](https://github.com/zeroroot-ai/adk/commit/c10f7bebd39e589fe1864325ced0fc839455b0cb)), closes [#185](https://github.com/zeroroot-ai/adk/issues/185)
* **mission:** check the target before anything is created server-side ([#181](https://github.com/zeroroot-ai/adk/issues/181)) ([fad5e2d](https://github.com/zeroroot-ai/adk/commit/fad5e2dd8467aca15579172cebda1957ede6211d)), closes [#179](https://github.com/zeroroot-ai/adk/issues/179)
* **mission:** submit now binds a target and actually runs the mission ([#176](https://github.com/zeroroot-ai/adk/issues/176)) ([0ec8d57](https://github.com/zeroroot-ai/adk/commit/0ec8d579e41afba0a9a6da10b75024e55ef89ef0))

## [0.106.0](https://github.com/zeroroot-ai/adk/compare/v0.105.0...v0.106.0) (2026-06-29)


### ⚠ BREAKING CHANGES

* **cli:** narrow gibson-cli to the narrowed SDK — drop provider + mission draft ([#164](https://github.com/zeroroot-ai/adk/issues/164))
* **module:** move the Go module to gibson/ so its import path matches its location (closes #150) ([#151](https://github.com/zeroroot-ai/adk/issues/151))
* **cli:** `gibson mission submit`/`draft` drop --daemon / GIBSON_DAEMON_ADDR / --insecure / --tenant <addr> / --tenant-id / GIBSON_TENANT_ID; the daemon URL comes from the login session (override with --gibson-url) and --tenant now means a tenant id.
* **cli:** `gibson target` drops --daemon / GIBSON_DAEMON_ADDR and --insecure; the daemon URL comes from the login session (override with --gibson-url) and --tenant now means a tenant id.
* **cli:** `gibson provider` drops the `--insecure` and `--tenant <grpc-address>` / GIBSON_TENANT_ADDR flags; `--tenant` now means a tenant id and the daemon URL comes from the login session (override with `--gibson-url`).
* **deps:** consume sdk v0.129.1 (ADR-0039 service reorg) ([#107](https://github.com/zeroroot-ai/adk/issues/107))

### Features

* add gibson login/logout via Zitadel device authorization grant ([#117](https://github.com/zeroroot-ai/adk/issues/117)) ([7d30282](https://github.com/zeroroot-ai/adk/commit/7d3028251c9b7489a535b9c0b556e8e0af2abce9))
* **agent:** surface bootstrap_token in `gibson agent enroll` output (gibson[#648](https://github.com/zeroroot-ai/adk/issues/648)) ([#128](https://github.com/zeroroot-ai/adk/issues/128)) ([049b310](https://github.com/zeroroot-ai/adk/commit/049b310f4b67395527f16b26a03c60d36ba14486))
* authed CLI session middleware + tenant resolution; agent enroll via Envoy ([#119](https://github.com/zeroroot-ai/adk/issues/119)) ([9093394](https://github.com/zeroroot-ai/adk/commit/90933940b27cad6c2f9dc2279751d116aaee79cc))
* **cli:** delete the gibson connector verbs; gibson component handles runtime: mcp-bridge plugins ([#153](https://github.com/zeroroot-ai/adk/issues/153)) ([342159a](https://github.com/zeroroot-ai/adk/commit/342159a6086d33a8443a635ab073ebc9c2d75818))
* **cli:** gibson target create/list/get/update/delete commands ([#97](https://github.com/zeroroot-ai/adk/issues/97)) ([1ca4268](https://github.com/zeroroot-ai/adk/commit/1ca42682915f292fdb5174c83e1ff013871055ee))
* **cli:** migrate gibson mission submit/draft onto authenticated daemon channel ([#141](https://github.com/zeroroot-ai/adk/issues/141)) ([542621b](https://github.com/zeroroot-ai/adk/commit/542621ba58a818f70f832db5f703a29c82a493c8)), closes [#138](https://github.com/zeroroot-ai/adk/issues/138)
* **cli:** migrate gibson provider to authenticated daemon channel ([#139](https://github.com/zeroroot-ai/adk/issues/139)) ([983d9e5](https://github.com/zeroroot-ai/adk/commit/983d9e597e46baa81c4a4241721f0df3d31de746)), closes [#136](https://github.com/zeroroot-ai/adk/issues/136)
* **cli:** migrate gibson target onto authenticated daemon channel ([#140](https://github.com/zeroroot-ai/adk/issues/140)) ([144e620](https://github.com/zeroroot-ai/adk/commit/144e6202637ab82b9b2a0b3787306d9fef3ce2fb)), closes [#137](https://github.com/zeroroot-ai/adk/issues/137)
* **cli:** narrow gibson-cli to the narrowed SDK — drop provider + mission draft ([#164](https://github.com/zeroroot-ai/adk/issues/164)) ([edf065f](https://github.com/zeroroot-ai/adk/commit/edf065ff99a6726968d8e982718210164a20e5af))
* **cli:** register/inspect over Capability-Grant runtime credential (adk[#124](https://github.com/zeroroot-ai/adk/issues/124), adk[#125](https://github.com/zeroroot-ai/adk/issues/125)) ([#130](https://github.com/zeroroot-ai/adk/issues/130)) ([238fe92](https://github.com/zeroroot-ai/adk/commit/238fe92df2e03d7faacb0e0a8dbe5847faadd22f))
* **connector:** scaffold and locally run MCP connectors (adk[#142](https://github.com/zeroroot-ai/adk/issues/142)) ([#144](https://github.com/zeroroot-ai/adk/issues/144)) ([b554b55](https://github.com/zeroroot-ai/adk/commit/b554b55fd7b8bcd3d353611c38ec4b3d5910b552))


### Bug Fixes

* **ci:** bump go toolchain to 1.25.11 to clear stdlib govulncheck advisories ([#120](https://github.com/zeroroot-ai/adk/issues/120)) ([8fd6cb9](https://github.com/zeroroot-ai/adk/commit/8fd6cb9d389617d1a57ac810e3ba5650ef545a78)), closes [#118](https://github.com/zeroroot-ai/adk/issues/118)
* **ci:** gofmt drifted cli files and gate formatting in CI (adk[#145](https://github.com/zeroroot-ai/adk/issues/145)) ([#147](https://github.com/zeroroot-ai/adk/issues/147)) ([769144d](https://github.com/zeroroot-ai/adk/commit/769144d08be2153c4a06568b6ab7d70dc1ba1bab))
* **cli:** component build resolves modules on first build (go mod tidy) ([#111](https://github.com/zeroroot-ai/adk/issues/111)) ([9c6bdc3](https://github.com/zeroroot-ai/adk/commit/9c6bdc36b71280bbf8f1c82b13e4d0104ad2ab59)), closes [#110](https://github.com/zeroroot-ai/adk/issues/110)
* **cli:** print only the bootstrap token from agent enroll (CG-only) ([#135](https://github.com/zeroroot-ai/adk/issues/135)) ([d525742](https://github.com/zeroroot-ai/adk/commit/d52574236794a9462365c0b3415d855aec732992))
* **deps:** consume sdk v0.129.1 (ADR-0039 service reorg) ([#107](https://github.com/zeroroot-ai/adk/issues/107)) ([32125e2](https://github.com/zeroroot-ai/adk/commit/32125e29300ed7bc96ba37f7d13d6259f3bbd10a))
* **deps:** update first-party deps to post-rename module path versions ([#83](https://github.com/zeroroot-ai/adk/issues/83)) ([1d20639](https://github.com/zeroroot-ai/adk/commit/1d20639f32b6105509453b7b1894ab24a7075d5e))
* lay scaffolded protos at package-matching paths for buf STANDARD ([#92](https://github.com/zeroroot-ai/adk/issues/92)) ([00825d2](https://github.com/zeroroot-ai/adk/commit/00825d25b22c9d14ed50d1f7c6c42185c44e1bc4))
* make gibson component init scaffolds build, generate, and validate ([#102](https://github.com/zeroroot-ai/adk/issues/102)) ([44f0344](https://github.com/zeroroot-ai/adk/commit/44f0344b8595c55dacdd926b8f001ef3a57e2bf5))
* **module:** move the Go module to gibson/ so its import path matches its location (closes [#150](https://github.com/zeroroot-ai/adk/issues/150)) ([#151](https://github.com/zeroroot-ai/adk/issues/151)) ([34c3ba7](https://github.com/zeroroot-ai/adk/commit/34c3ba78b8402b6dc61b9b80f88423a6257fc315))
* regenerate embedded mission CUE from SDK proto (llm_slots) ([#104](https://github.com/zeroroot-ai/adk/issues/104)) ([7845ae0](https://github.com/zeroroot-ai/adk/commit/7845ae07bceaa78da7a59e194be3c70bba1fd4fb))
* sanitize component names into Go/proto identifiers in scaffolds ([#90](https://github.com/zeroroot-ai/adk/issues/90)) ([e6146b8](https://github.com/zeroroot-ai/adk/commit/e6146b8c3a60b7375c7801070acb754e5160beb0)), closes [#89](https://github.com/zeroroot-ai/adk/issues/89)
* **scaffold:** rewrite templates to the Observe emit model (closes [#156](https://github.com/zeroroot-ai/adk/issues/156)) ([#157](https://github.com/zeroroot-ai/adk/issues/157)) ([6380104](https://github.com/zeroroot-ai/adk/commit/63801041096858033bc5604d48124bec11a44c18))
* **scaffold:** unify component enrollment on capability-grant (ADR-0045) ([#134](https://github.com/zeroroot-ai/adk/issues/134)) ([214f90f](https://github.com/zeroroot-ai/adk/commit/214f90f0a33916f40f9d9a4e695fc91c8c266ceb))
* update scaffold templates domain from zero-day.ai to zeroroot.ai ([#85](https://github.com/zeroroot-ai/adk/issues/85)) ([d17da3c](https://github.com/zeroroot-ai/adk/commit/d17da3cc2688dc699dc3195069491d7447e349ae))

## [0.105.0](https://github.com/zeroroot-ai/adk/compare/v0.104.3...v0.105.0) (2026-05-24)


### Features

* **cli:** mission submit full CUE→define→run; add draft, provider, agent subcommands ([#68](https://github.com/zeroroot-ai/adk/issues/68)) ([25c0f7e](https://github.com/zeroroot-ai/adk/commit/25c0f7eb1cc36fde773440f4ec311b322390b37d)), closes [#66](https://github.com/zeroroot-ai/adk/issues/66)


### Bug Fixes

* **ci:** remove PR trigger and use security-extended for CodeQL ([#72](https://github.com/zeroroot-ai/adk/issues/72)) ([bab5c15](https://github.com/zeroroot-ai/adk/commit/bab5c152f1e7f643b47a6b802b15b7d526760092)), closes [#71](https://github.com/zeroroot-ai/adk/issues/71)

## [0.104.3](https://github.com/zeroroot-ai/adk/compare/v0.104.2...v0.104.3) (2026-05-24)


### Bug Fixes

* **cli:** replace os.Exit in cmd handlers with ExitCodeError return ([#60](https://github.com/zeroroot-ai/adk/issues/60)) ([4b49884](https://github.com/zeroroot-ai/adk/commit/4b4988463763ab0018b7599bb897e1cc00bdf666))
* **deps:** bump golang.org/x/net v0.53.0 → v0.55.0 (GO-2026-5026) ([#58](https://github.com/zeroroot-ai/adk/issues/58)) ([2e7edfd](https://github.com/zeroroot-ai/adk/commit/2e7edfd730de45ecfa4983d786bba332fef3ee6a))
* **deps:** update sdk from stale v1.9.0 to v0.114.1 after polyrepo version reset ([#62](https://github.com/zeroroot-ai/adk/issues/62)) ([5db2557](https://github.com/zeroroot-ai/adk/commit/5db255780499470f2d8c27e869c66e515308bd7c))

## [0.104.2](https://github.com/zeroroot-ai/adk/compare/v0.104.1...v0.104.2) (2026-05-20)


### Bug Fixes

* stabilise templates-export by routing through encoding/json.Indent ([#35](https://github.com/zeroroot-ai/adk/issues/35)) ([e93e9d9](https://github.com/zeroroot-ai/adk/commit/e93e9d93c6a89e6329ce10e42d2563df259a5dae))

## [0.104.1](https://github.com/zeroroot-ai/adk/compare/v0.104.0...v0.104.1) (2026-05-17)


### Bug Fixes

* regen templates/*/template.json to drop double-space after colons ([#30](https://github.com/zeroroot-ai/adk/issues/30)) ([3e73160](https://github.com/zeroroot-ai/adk/commit/3e731604c632e4a60f288d3a51edd10e963f27fa))

## [1.2.0](https://github.com/zeroroot-ai/adk/compare/v1.1.0...v1.2.0) (2026-05-13)


### Features

* **scaffold:** auto-wire OntologyContributor for tool scaffolds ([#15](https://github.com/zeroroot-ai/adk/issues/15)) ([f36db9b](https://github.com/zeroroot-ai/adk/commit/f36db9b158ab63079a65d8c7b85986e93913a90f))

## [1.1.0](https://github.com/zeroroot-ai/adk/compare/v1.0.0...v1.1.0) (2026-05-13)


### Features

* add ontology codegen and component scaffold updates ([#9](https://github.com/zeroroot-ai/adk/issues/9)) ([5a19643](https://github.com/zeroroot-ai/adk/commit/5a1964386bc4d01fba577f418858b738c83f7d7a))


### Bug Fixes

* **ci:** adk CI hygiene cleanup ([#5](https://github.com/zeroroot-ai/adk/issues/5)) ([e3d48aa](https://github.com/zeroroot-ai/adk/commit/e3d48aa0c9dedf12fcec5794f560af121937b87f))

## 1.0.0 (2026-05-10)


### Features

* add CI, CodeQL, and Scorecard workflows ([#1](https://github.com/zeroroot-ai/adk/issues/1)) ([8cc9f59](https://github.com/zeroroot-ai/adk/commit/8cc9f59e3e81c49b42e827da19fe791fbf05b2b5))
* install release-please and pr-title-lint ([#2](https://github.com/zeroroot-ai/adk/issues/2)) ([ed5c342](https://github.com/zeroroot-ai/adk/commit/ed5c3423ac7d4b34fa2a70f77108cf13b04a9b28))

## ADK Changelog

## Unreleased

The ADK CLI is the canonical Gibson developer workflow tool —
scaffolds, validates, registers, and runs Gibson components, with AI-
coder context (`AGENTS.md`, `CLAUDE.md`, `prompts/`) baked into every
scaffold so an LLM coder can be productive on first open. Spec:
[`.spec-workflow/specs/adk-developer-workflow/`](../../.spec-workflow/specs/adk-developer-workflow/).

### Verbs

```
gibson init                              # workspace bootstrap
gibson component init <name> --kind agent|tool|plugin
gibson component validate
gibson component register
gibson component run
gibson docs schema [component-yaml | plugin-yaml]
gibson inspect
```

### Highlights

- **Scaffolds for all three component shapes.** Tool scaffold ships
  proto field 100 = `gibson.graphrag.v1.DiscoveryResult` plus
  `buf.yaml`, `buf.gen.yaml`, and vendored SDK protos so `make proto`
  works out of the box. Plugin scaffold has the same buf vendoring.
  Agent scaffold ships the LLM-slot + harness skeleton.

- **`AGENTS.md` per kind, grounded in real SDK source paths**, verified
  by a link-resolution test (`TestAgentsMD_LinksResolveAgainstLocalSDK`)
  that walks every `core/sdk/...` reference and asserts the file
  exists at the pinned SDK tag.

- **Workspace config (`.gibson/workspace.yaml`)** refuses
  credential-named fields and world-writable mode permissions.
  Carries no tenant identifier — tenant context is embedded in the
  credentials the dashboard issues.

- **`gibson docs schema`** emits JSON Schema (Draft 2020-12) for
  `component.yaml` and `plugin.yaml` so editors and AI coders get
  inline validation.

- **Process supervisor** (`internal/runner`) forwards SIGINT/SIGTERM
  with `--drain-timeout` and surfaces exit code 75 (the SDK plugin
  rotation contract) verbatim.

- **No admin RPCs.** `gibson component register` is a paste-the-
  `enroll_command` consumer, by design. Identity minting stays in the
  dashboard.

- **No back-compat shims.** Clean cutover: `gibson plugin <verb>` /
  `gibson agent enroll` / `gibson tool enroll` (the pre-spec verb
  forms) are gone. Update Makefiles and CI to use
  `gibson component <verb>`.

- **Integration tests behind `//go:build integration`.** Per kind:
  render scaffold, `go mod tidy`, `go build` — and for tool / plugin,
  `buf generate` + grep for `Discovery *DiscoveryResult` with proto
  tag 100 in the generated `.pb.go`. Run via
  `make test-integration`.

- **Golden-file scaffold tests for all three kinds.** Drift fails CI;
  intentional changes regenerate via `make update-golden`.

### Migration

Pre-spec scaffolds and Makefiles will need updates:

- `gibson-cli plugin enroll --token T` → `gibson component register --token T`
- `gibson-cli agent enroll …` → `gibson component register …`
- `gibson-cli tool enroll …`  → `gibson component register …`

For plugin scaffolds without `buf.yaml`, re-init via
`gibson component init <name> --kind plugin --force` (or hand-add
`buf.yaml` / `buf.gen.yaml` / `proto/vendor/`).

Requires `github.com/zeroroot-ai/sdk` v1.2.0+.
