# ClickFix SDKs (unpublished)

- `go`: generated server client plus authentication, bounded retries, pagination and webhook verification.
- `node`: typed, tree-shakable Node 22+ fetch client and server cryptographic helpers.
- `react-widget`: UI-free React 18+ visitor client and hooks; browser requests use the same-origin BFF.

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
make sdks-check                 # compares files without git metadata or mutations
(cd sdks && pnpm test)
(cd sdks/go && go test -p 2 ./...)
```

Generation is checked in a separate required CI job so generator installation does
not slow the existing contracts job. Go SDK security and SDK dependency audits run
in the required `security` job. No package is published; all package manifests
remain private. Build output and dependencies are ignored.

References: existing ClickFix widget routes/types and Laravel
`resources/js/widget/lib/widget-api.ts` for the headless transport approach;
Rentopia `backend/internal/modules/publicapi/usecase/webhooks.go` for the timestamp.body
HMAC format, adapted to ClickFix's existing delivery contract.
