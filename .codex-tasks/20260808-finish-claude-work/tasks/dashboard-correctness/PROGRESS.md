# Progress Log

## Context Recovery Block

- **Current milestone**: #5 - Run dashboard validation
- **Current status**: DONE
- **Last completed**: #5 - Dashboard correctness validation passed.
- **Current artifact**: `TODO.csv`
- **Key context**: Provider aggregation drops cache fields; History API overloads `input_tokens`; non-streaming recording drops cache usage; History detail interaction is pointer-only and does not manage dialog focus.
- **Known issues**: Frontend interaction has no browser test harness, so native behavior needs static checks and a focused manual/demo flow.
- **Next action**: Return to the parent Epic and begin the scoped sub2api presentation port.

## Milestone 1: Trace token contracts and History interaction

- **Status**: DONE
- **Completed**: 01:43
- **Key decisions**:
  - `input_tokens` remains raw non-cache input everywhere.
  - `prompt_tokens` is the explicit UI total: raw input + cache read + cache creation.
  - Use native `<dialog>` instead of writing a custom focus trap.
- **Next step**: Milestone 2 - Fix provider and History token contracts.

## Milestone 2: Fix provider and History token contracts

- **Status**: DONE
- **Completed**: 01:46
- **Validation**:
  - `go test ./internal/storage -run TestGetProviderBreakdown -count=1` -> pass.
  - `go test ./internal/gui -run TestHistoryEntries -count=1` -> pass.
- **Next step**: Milestone 3 - Record non-streaming cache usage.

## Milestone 3: Record non-streaming cache usage

- **Status**: DONE
- **Completed**: 01:48
- **Validation**:
  - `go test ./internal/handlers -run TestDecodeMessageUsageIncludesCacheTokens -count=1` -> pass.
  - `go vet ./internal/handlers` -> pass.
- **Next step**: Milestone 4 - Add keyboard and modal focus behavior.

## Milestone 4: Add keyboard and modal focus behavior

- **Status**: DONE
- **Completed**: 01:53
- **What was done**:
  - Added keyboard activation to History rows and sortable headers.
  - Replaced the custom detail overlay with native `<dialog>` focus behavior.
  - Added prompt/raw/cache token detail and prompt-token CSV output.
- **Validation**:
  - `node --check internal/gui/assets/app.js` -> pass.
  - `go test ./internal/gui -run 'TestHistory(Entries|Assets)' -count=1` -> pass.
- **Next step**: Milestone 5 - Run dashboard validation.

## Milestone 5: Run dashboard validation

- **Status**: DONE
- **Completed**: 01:54
- **Validation**:
  - `go test ./internal/storage ./internal/gui ./internal/handlers -count=1` -> pass.
  - `go vet ./internal/storage ./internal/gui ./internal/handlers` -> pass.
  - `gofmt -l internal/storage internal/gui internal/handlers` -> no output.
  - `node --check internal/gui/assets/app.js` -> pass.
- **Next step**: Parent Epic child #4.
