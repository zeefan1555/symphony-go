# Use gen/hertz For The Generated Control Plane Shell

Status: accepted

Symphony Go keeps Hertz-managed HTTP code in `gen/hertz/...` as the long-term generated product control plane shell. `idl/main.proto` remains the only Hertz service entry and the source of route annotations, and `buf lint` is the required automated IDL check before generation. Generated handler, model, and router code must be reproducible from the Hertz generation command and must not contain hand-written business behavior.

This supersedes earlier wording that treated a root-level generated tree as the Hertz shell. The current generated tree is `gen/hertz/...`.

## Consequences

- `gen/hertz/...` is the long-term generated code root for Hertz handler, model, and router output.
- `internal/transport/hertzbinding` is the hand-written transport binding Module that connects generated handlers to the control service and owns HTTP error envelope behavior.
- `internal/service/control` is the hand-written business entry for stable product control plane semantics.
- The generated tree must not import `internal/service/...` business packages directly except through the approved hand-written binding path.
- Hand-written business logic must not be added to generated handler, model, or router files.
- 旧 scaffold packages and old scaffold routes are retired design artifacts. Future work must not add new `internal/service/*/scaffold` packages.
- A single implementation does not justify a public Adapter-shaped seam. Avoid `Adapter` naming unless a real seam has multiple concrete implementations.
- Existing references should use `gen/hertz/...` for Hertz-generated handler, model, and router code.
