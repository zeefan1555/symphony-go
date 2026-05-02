GO ?= go
BINARY := bin/symphony-go
WORKFLOW ?= ./WORKFLOW.md
ZH_SMOKE_WORKFLOW ?= ./WORKFLOW.zh-smoke.md
ISSUE ?=
MERGE_TARGET ?=
MERGE_TARGET_FLAG := $(if $(strip $(MERGE_TARGET)),--merge-target $(MERGE_TARGET),)
LOG ?=
RESULTS ?= ../.codex/skills/zh-smoke-harness/experiments/results.tsv
RESULTS_MD ?= ../.codex/skills/zh-smoke-harness/experiments/rounds.md
TEAM ?= Zeefan
CHANGE_NOTE ?=
ZH_SMOKE_PROCESS_PATTERN ?= symphony-go.*run.*WORKFLOW.zh-smoke.md

# External link mode keeps macOS binaries dyld-friendly on machines that reject
# some `go run` temporary executables with a missing LC_UUID load command.
LDFLAGS := -linkmode=external

.PHONY: build test run run-once zh-smoke-run zh-smoke-once zh-smoke-stop zh-smoke-metrics zh-smoke-round clean

build:
	GO=$(GO) SYMPHONY_GO_BUILD_LDFLAGS="$(LDFLAGS)" SYMPHONY_GO_BINARY="$(BINARY)" ./build.sh

test:
	GO=$(GO) SYMPHONY_GO_TEST_LDFLAGS="$(LDFLAGS)" ./test.sh ./...

run: build
	$(BINARY) run --workflow $(WORKFLOW) --tui $(MERGE_TARGET_FLAG)

# Debug helper for a single poll. Production usage should prefer `make run`.
run-once: build
	$(BINARY) run --workflow $(WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),) $(MERGE_TARGET_FLAG)

zh-smoke-run: build
	$(BINARY) run --workflow $(ZH_SMOKE_WORKFLOW) --tui $(MERGE_TARGET_FLAG)

zh-smoke-once: build
	$(BINARY) run --workflow $(ZH_SMOKE_WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),) $(MERGE_TARGET_FLAG)

zh-smoke-stop:
	-pkill -f "$(ZH_SMOKE_PROCESS_PATTERN)"

zh-smoke-metrics:
	python3 scripts/smoke_metrics.py $(if $(LOG),--log $(LOG),) $(if $(ISSUE),--issue $(ISSUE),) --append $(RESULTS) --markdown-append $(RESULTS_MD) $(if $(CHANGE_NOTE),--change-note "$(CHANGE_NOTE)",)

zh-smoke-round:
	python3 scripts/zh_smoke_round.py --team $(TEAM) --workflow $(ZH_SMOKE_WORKFLOW) $(if $(MERGE_TARGET),--merge-target $(MERGE_TARGET),) --results $(RESULTS) --markdown $(RESULTS_MD) $(if $(CHANGE_NOTE),--change-note "$(CHANGE_NOTE)",)

clean:
	rm -rf bin
