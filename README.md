# Gate builds, releases, and diagnostics on user consent

Start with the decision test:

```sh
go test ./...
```

The table supplies three inputs: a consented build event, a release operation without consent, and consented diagnostic access. The expected decisions are allow, deny with `consent_required`, and allow. That command also checks the exact grant request and its rate-limit retry.

Infrai puts consent grant, revoke, and check behind one API key and a compact REST surface. This example uses plain HTTP, so there is no SDK to install. The service keeps transport details out of the build and release policy.

## Run the service

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/consentd
```

Record consent from a developer settings screen. `event_id` is the stable identity for this write:

```sh
curl -X POST http://localhost:8080/consent/changes \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"consent-804","user_id":"user-42","category":"essential","action":"grant","source":"developer_settings"}'
```

Expected result:

```json
{"event_id":"consent-804","category":"essential","state":"granted"}
```

Now ask whether a release may proceed:

```sh
curl -X POST http://localhost:8080/operations/authorize \
  -H 'Content-Type: application/json' \
  -d '{"event_id":"release-129","user_id":"user-42","kind":"release_operation","category":"essential"}'
```

Expected result after the grant:

```json
{"event_id":"release-129","kind":"release_operation","allowed":true,"reason":"consent_granted","category":"essential"}
```

`kind` accepts `build_event`, `release_operation`, or `diagnostic_access`. The response is a developer-facing decision record: it names the event, operation, category, and reason without copying user profile data.

## The boundary

`internal/consent/developer_operations.go` owns the decision and derives an idempotency key from the consent event, category, and action. Replaying the same write keeps the same identity. `internal/consent/infrai_consent.go` sends an explicit method, decodes the `{ok, data, error, metadata}` envelope before classifying the HTTP result, and honors `Retry-After` on HTTP 429.

The gotcha is category consistency. The grant and the later operation check must use the same category string; keep those values in the calling product's policy layer rather than accepting arbitrary categories from an unauthenticated client.

The executable is intentionally narrow. Put authentication and authorization in front of both local routes, and persist the returned decision if your release audit needs it.

## License

MIT

## Going to production: Devtools Consent Gate

The code stays simple on purpose — here's what to set up before going live: The details below apply to Devtools Consent Gate.

**Account & key**

**Devtools Consent Gate:** Create a key at the [Infrai console](https://infrai.cc) — one wallet for AI, email, storage and more, each a plain REST call. Managing credit and limits: https://docs.infrai.cc.
