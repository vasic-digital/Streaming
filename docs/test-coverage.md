# test-coverage.md — digital.vasic.streaming

Round 284 symbol → test / Challenge ledger. Every exercised public
symbol of `digital.vasic.streaming` listed here with the test(s)
and Challenge(s) that verify it AND the anti-bluff dimension each
proves. Adding an exported symbol without updating this ledger is
a CONST-048 violation. Per Article XI §11.9, every PASS row MUST
carry positive runtime evidence — the "Evidence" column documents
what to capture during a release-gate sweep.

## Components exercised by the runner

| Component             | Exported symbols                                                                 | Unit test(s) location           | Challenge component name in runner | Anti-bluff dimension                                                                          | Evidence (runtime)                                              |
|-----------------------|----------------------------------------------------------------------------------|----------------------------------|--------------------------------------|-----------------------------------------------------------------------------------------------|------------------------------------------------------------------|
| SSE broker            | `Broker`, `NewBroker`, `Broker.AddClient`, `Broker.Broadcast`, `Broker.ClientCount`, `Broker.Close`, `Event` | `pkg/sse/sse_test.go`            | `sse-broker`                         | Real `httptest.Server` round-trip — Broadcast actually serialised to wire bytes the client reads. | `component=sse-broker status=PASS detail="client_count=1 wire_bytes=N"`. |
| Webhook signing       | `Sign`, `Verify`                                                                 | `pkg/webhook/webhook_test.go`    | `webhook-sign-verify`                | Real HMAC-SHA256 — tamper of one byte rejected; matching sig accepted.                          | `component=webhook-sign-verify status=PASS detail="sig_len=N tamper_rejected=true"`. |
| HTTP circuit breaker  | `NewClient`, `Client.Request`, `Client.CircuitState`, `CircuitOpen`              | `pkg/http/http_test.go`          | `http-circuit-breaker`               | Real failing server (`httptest`) drives breaker `Closed → Open` after threshold.                | `component=http-circuit-breaker status=PASS detail="circuit=open hits=N"`. |
| Transport factory     | `NewFactory`, `Factory.SupportedTypes`                                           | `pkg/transport/transport_test.go`| `transport-factory`                  | Built-in registration produces non-empty closed type set.                                       | `component=transport-factory status=PASS detail="supported_types=N"`. |
| Realtime debouncer    | `NewDebouncer`, `Debouncer.Start`, `Debouncer.Notify`, `Debouncer.Stop`, `ChangeEvent`, `ChangeType` | `pkg/realtime/realtime_test.go` | `realtime-debouncer`                 | Two `Notify` calls inside one window deliver as a single batch to the handler.                  | `component=realtime-debouncer status=PASS detail={"batches":N,"len":N}`. |

## Anti-bluff dimensions covered

| Dimension                                                          | Where proved                                                  |
|--------------------------------------------------------------------|---------------------------------------------------------------|
| Real I/O (no mocks beyond unit tests, per CONST-050(A))            | Runner uses real `httptest.Server` + real broker + real HMAC. |
| Wire-level observable behaviour (Article XI §11.9)                 | SSE bytes read off the socket; webhook tamper byte mutated.   |
| Closed-set coverage assertion (CONST-048)                          | Runner refuses to PASS if `len(checks) != len(expectedComponents)`. |
| 5-locale bilingual UX (CONST-046)                                  | Runner prints `en/sr/ja/es/de` summary lines.                 |
| Paired-mutation evidence (CONST-035)                               | `--mutate` flag builds scratch breaker that fails to open.    |
| No fakes beyond unit tests (CONST-050(A))                          | No `internal/mocks`, no test doubles — real packages only.    |
| Error path surfaced loudly (no silent miss)                        | Sentinel exit codes 2/3/4 for coverage/invariant/locale gaps. |

## Maintenance

Every CL that adds, removes, or renames an exported symbol on any
listed component MUST update this ledger AND the runner's
`expectedComponents` list in the SAME commit. Drift is a CONST-048
violation. Adding a NEW component to a `pkg/` package without a
corresponding row + runner check is a CONST-050(B) coverage gap.

Per CONST-049, governance edits flow through the constitution
submodule's 7-step pipeline; this file is project-side documentation
and does NOT require the constitution-update workflow for routine
ledger maintenance, but DOES require the workflow when reshaping
the anti-bluff dimension columns to add a new constitutional anchor.
