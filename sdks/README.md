# ClickFix SDKs

MIT licensed (`LICENSE`, copied into every package). Published from the public mirror
[ClickSolveMainTR/clickfix-sdks](https://github.com/ClickSolveMainTR/clickfix-sdks): every tagged `main`
run pushes this directory there and tags each package version (`node/vX.Y.Z`, `go/vX.Y.Z`,
`flutter/vX.Y.Z`, ...). The mirror is read-only; fixes land here and ship with the next release.

- `go`: generated server client plus authentication, bounded retries, pagination and webhook verification. Module `github.com/ClickSolveMainTR/clickfix-sdks/go`, tags `go/vX.Y.Z` (version in `go/VERSION`).
- `node`: `@clickfix/node`, typed, tree-shakable Node 22+ fetch client and server cryptographic helpers.
- `react-widget`: `@clickfix/react-widget`, UI-free React 18+ visitor client and hooks; browser requests use the same-origin BFF.
- `flutter/clickfix_webview`: `clickfix_webview` (pub.dev), Flutter webview widget + controller implementing the webview bridge contract v1 (`docs/webview-bridge.md`). Checked by the separate `flutter` CI job (`dart analyze`, `flutter test`, publish dry run); published by `sdk-publish` when the `PUB_CREDENTIALS` secret is set.
- `examples`: the integration guide's samples (widget JS, Node, Go, curl) and the runner that verifies them (`examples/README.md`).

The source of truth is `backend/docs/openapi.yaml`. `make sdks` selects only
`/v1/public-api/*` for server clients and `/v1/widget/*` for the visitor client,
retains referenced components transitively, and generates Go with
[oapi-codegen v2.8.0](https://github.com/oapi-codegen/oapi-codegen/releases/tag/v2.8.0)
and TypeScript with [openapi-typescript 7.13.0](https://openapi-ts.dev/node).
Go and Node cover the API-key integration routes (including triggers and subscriptions);
JWT-authenticated key administration is deliberately outside a server SDK.
The existing API has some generic response envelopes; their payloads remain unknown
until the canonical contract gives them a concrete schema. Do not cast unvalidated input.

Requires Go matching `backend/go.mod`, Node 22+ and pnpm 11.22.0.

```sh
make sdks
make sdks-check                 # generated files current + guide pages current
(cd sdks && pnpm test)
(cd sdks/go && go test -p 2 ./...)
make guide-examples             # run the guide samples (stub API, or CLICKFIX_BASE_URL)
```

Generation is checked in a separate required CI job so generator installation does
not slow the existing contracts job. Go SDK security and SDK dependency audits run
in the required `security` job.

## Yayın (CF-199)

Versions live in each package (`node/package.json`, `react-widget/package.json`,
`go/VERSION`) next to its `CHANGELOG.md`. On every PR the `sdks` job runs
`npm pack --dry-run` and `npm publish --dry-run`. On a green `main` the
`sdk-publish` job publishes any npm version that is not on the registry yet
(needs the `NPM_TOKEN` repository secret, an npm automation token for the
`@clickfix` scope; without it the job skips with a note), and the `sdks-mirror`
job pushes the tree and the `go/vX.Y.Z` tag to the public mirror when they do not exist.
Nobody publishes or tags by hand. Details: `docs/entegrasyon-kilavuzu.md`.

References: existing ClickFix widget routes/types and Laravel
`resources/js/widget/lib/widget-api.ts` for the headless transport approach;
Rentopia `backend/internal/modules/publicapi/usecase/webhooks.go` for the timestamp.body
HMAC format, adapted to ClickFix's existing delivery contract.
