GO ?= go
BINARY := bin/symphony-go
WORKFLOW ?= ./workflows/WORKFLOW-symphony-go.md
BYTECODE_WORKFLOW ?= ./workflows/WORKFLOW-bytedcode.md
EXPLORE_WORKFLOW ?= ./workflows/WORKFLOW-zeefan-explore.md
ZEEFAN_EXPLORE_WORKFLOW ?= /Users/bytedance/zeefan-explore/WORKFLOW.md
ZH_SMOKE_WORKFLOW ?= $(WORKFLOW)
ISSUE ?=
MERGE_TARGET ?=
MERGE_TARGET_FLAG := $(if $(strip $(MERGE_TARGET)),--merge-target $(MERGE_TARGET),)
LOG ?=
RESULTS ?= ../.codex/skills/zh-smoke-harness/experiments/results.tsv
RESULTS_MD ?= ../.codex/skills/zh-smoke-harness/experiments/rounds.md
TEAM ?= Zeefan
CHANGE_NOTE ?=
BYTECODE_TRIAGE_PROCESS_PATTERN ?= symphony-go.*run.*workflows/WORKFLOW-bytedcode.md
EXPLORE_PROCESS_PATTERN ?= symphony-go.*run.*workflows/WORKFLOW-zeefan-explore.md
ZEEFAN_EXPLORE_PROCESS_PATTERN ?= symphony-go.*run.*$(ZEEFAN_EXPLORE_WORKFLOW)
ZH_SMOKE_PROCESS_PATTERN ?= symphony-go.*run.*workflows/WORKFLOW-symphony-go.md

# External link mode keeps macOS binaries dyld-friendly on machines that reject
# some `go run` temporary executables with a missing LC_UUID load command.
LDFLAGS := -linkmode=external

.PHONY: build test idl-lint hertz-generate check-generated-hertz-boundary hertz-layout-smoke run run-once bytecode bytecode-once bytecode-stop bytecode-triage bytecode-triage-once bytecode-triage-stop explore explore-once explore-stop zeefan-explore zeefan-explore-once zeefan-explore-stop zh-smoke-run zh-smoke-once zh-smoke-stop zh-smoke-metrics zh-smoke-round clean

build:
	GO=$(GO) SYMPHONY_GO_BUILD_LDFLAGS="$(LDFLAGS)" SYMPHONY_GO_BINARY="$(BINARY)" ./build.sh

test:
	GO=$(GO) SYMPHONY_GO_TEST_LDFLAGS="$(LDFLAGS)" ./test.sh ./...

idl-lint:
	buf lint

hertz-generate: idl-lint
	scripts/hertz_generate.sh

check-generated-hertz-boundary:
	scripts/check_generated_hertz_boundary.sh

hertz-layout-smoke: build check-generated-hertz-boundary
	GO=$(GO) SYMPHONY_GO_TEST_LDFLAGS="$(LDFLAGS)" ./test.sh ./internal/runtime/config ./internal/app ./internal/integration/linear ./internal/service/control ./internal/transport/hertzserver ./internal/hertzcontract

run: build
	scripts/run_named_workflow.sh $(BINARY) $(WORKFLOW) --tui $(MERGE_TARGET_FLAG)

# Debug helper for a single poll. Production usage should prefer `make run`.
run-once: build
	scripts/run_named_workflow.sh $(BINARY) $(WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),) $(MERGE_TARGET_FLAG)

bytecode: bytecode-triage

bytecode-once: bytecode-triage-once

bytecode-stop: bytecode-triage-stop

bytecode-triage: build
	scripts/run_named_workflow.sh $(BINARY) $(BYTECODE_WORKFLOW) --no-tui $(if $(ISSUE),--issue $(ISSUE),)

bytecode-triage-once: build
	scripts/run_named_workflow.sh $(BINARY) $(BYTECODE_WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),)

bytecode-triage-stop:
	-pkill -f "$(BYTECODE_TRIAGE_PROCESS_PATTERN)"

explore: build
	scripts/run_named_workflow.sh $(BINARY) $(EXPLORE_WORKFLOW) --no-tui $(if $(ISSUE),--issue $(ISSUE),)

explore-once: build
	scripts/run_named_workflow.sh $(BINARY) $(EXPLORE_WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),)

explore-stop:
	-pkill -f "$(EXPLORE_PROCESS_PATTERN)"

zeefan-explore: build
	scripts/run_named_workflow.sh $(BINARY) $(ZEEFAN_EXPLORE_WORKFLOW) --no-tui $(if $(ISSUE),--issue $(ISSUE),)

zeefan-explore-once: build
	scripts/run_named_workflow.sh $(BINARY) $(ZEEFAN_EXPLORE_WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),)

zeefan-explore-stop:
	-pkill -f "$(ZEEFAN_EXPLORE_PROCESS_PATTERN)"

zh-smoke-run: build
	scripts/run_named_workflow.sh $(BINARY) $(ZH_SMOKE_WORKFLOW) --tui $(MERGE_TARGET_FLAG)

zh-smoke-once: build
	scripts/run_named_workflow.sh $(BINARY) $(ZH_SMOKE_WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),) $(MERGE_TARGET_FLAG)

zh-smoke-stop:
	-pkill -f "$(ZH_SMOKE_PROCESS_PATTERN)"

zh-smoke-metrics:
	python3 scripts/smoke_metrics.py $(if $(LOG),--log $(LOG),) $(if $(ISSUE),--issue $(ISSUE),) --append $(RESULTS) --markdown-append $(RESULTS_MD) $(if $(CHANGE_NOTE),--change-note "$(CHANGE_NOTE)",)

zh-smoke-round:
	python3 scripts/zh_smoke_round.py --team $(TEAM) --workflow $(ZH_SMOKE_WORKFLOW) $(if $(MERGE_TARGET),--merge-target $(MERGE_TARGET),) --results $(RESULTS) --markdown $(RESULTS_MD) $(if $(CHANGE_NOTE),--change-note "$(CHANGE_NOTE)",)

clean:
	rm -rf bin
