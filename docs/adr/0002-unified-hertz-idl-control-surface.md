# Unified Hertz IDL Control Surface

Symphony Go will use `idl/main.thrift` as the only Hertz generation entry for business HTTP interfaces. The main IDL owns the single service and all route annotations, while flat domain IDL files define only dedicated `XxxReq`/`XxxResp` contracts and nested models; generated handlers must delegate to handwritten service implementations.

## Considered Options

- Keep separate control and scaffold generation entries: preserves the old distinction between external control API and internal scaffold models, but forces maintainers to remember which `hz` command and IDL file applies to each interface.
- Use a single main IDL that only includes child services: keeps child service ownership explicit, but Hertz does not register included child service routes from a master IDL in this workflow.
- Use a single main IDL service with flat child contract files: centralises the API surface and gives Hertz one authority for model generation, handler scaffold, and route registration.

## Consequences

All business HTTP routes registered from the main IDL use POST action-style paths so local operator, TUI, agent, and smoke-harness callers can share one calling convention. Internal scaffold capabilities become local diagnostic control-plane APIs rather than stable product APIs. Hertz owns interface contracts, route registration, generated models, and handler skeletons; handwritten service and transport layers remain the authority for business behaviour and side effects.
