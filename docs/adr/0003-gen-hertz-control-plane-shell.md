# Use gen/hertz For The Generated Control Plane Shell

Status: accepted

Symphony Go will move Hertz-managed HTTP code to `gen/hertz/...` as the long-term generated control plane shell. `idl/main.thrift` remains the only Hertz service entry and the source of route annotations. Generated handler, model, and router code must be reproducible from the Hertz generation command and must not contain hand-written business behavior.

This supersedes earlier wording that treated a root `biz/...` tree as the long-term generated shell. `biz/...` may appear as the current pre-migration Hertz output until the generated tree migration lands, but it is no longer the architectural target.

## Consequences

- `gen/hertz/...` is the long-term generated code root for Hertz handler, model, and router output.
- `internal/transport/hertzbinding` is the hand-written transport binding Module that connects generated handlers to the control service and owns HTTP error envelope behavior.
- `internal/service/control` is the hand-written business entry for diagnostic control plane semantics.
- The generated tree must not import `internal/service/...` business packages directly except through the approved hand-written binding path.
- Hand-written business logic must not be added to generated handler, model, or router files.
- 旧 scaffold packages and old scaffold routes are retired design artifacts. Future work must not add new `internal/service/*/scaffold` packages.
- A single implementation does not justify a public Adapter-shaped seam. Avoid `Adapter` naming unless a real seam has multiple concrete implementations.
- Existing `biz/...` references should be treated as migration debt for the `gen/hertz/...` move, not as current domain language.
