# Prefer Hertz For The First Control Plane

Symphony Go remains a long-running listener service that polls Linear and runs issue work through the orchestrator; external APIs are transport adapters over that core, not the core itself. For the first external control plane, we will use Hertz because the current SPEC already defines an HTTP/dashboard-oriented extension and the initial consumers are humans, browsers, curl, and local operator tooling. Shared IDL models should stay transport-neutral, while Hertz-specific route annotations belong in an HTTP-specific IDL layer so a future Kitex RPC adapter can reuse the same control semantics without moving orchestrator logic into transport code.

## Considered Options

- Start with Kitex: familiar RPC workflow and good fit for service-to-service calls, but it is less direct for the current dashboard and local HTTP API shape.
- Start with Hertz: matches the current HTTP control surface and keeps the first runtime aligned with browser/operator usage.

## Consequences

- Keep common issue, observability, and error models in shared IDL files.
- Put Hertz route annotations only in an HTTP-specific control IDL.
- Do not make Hertz or generated HTTP handlers own orchestration state.
- Add a Kitex-specific control IDL later only when there is a real RPC consumer.
