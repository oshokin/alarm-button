# alarm-button modernization report

## Baseline

BASELINE

Go:
  version: go1.27.1 linux/amd64

go mod verify:
  PASS

go test ./...:
  PASS
  packages: 27 (14 with tests/no-test-files mixed output)
  duration: 2.73s

go test -race ./...:
  PASS
  duration: 5.40s

go vet ./...:
  PASS

go build ./...:
  PASS

Also captured before changes:

- `go env`
- `git status --short`
- `go list -m all`
- `go list -m -u all`

## Toolchain changes

- `go.mod` upgraded from `go 1.25` to `go 1.27.1`.
- Go tool dependencies added via `tool (...)` directives:
  - `google.golang.org/protobuf/cmd/protoc-gen-go`
  - `google.golang.org/grpc/cmd/protoc-gen-go-grpc`
  - `golang.org/x/tools/cmd/goimports`
  - `golang.org/x/vuln/cmd/govulncheck`

## Dependency changes

Runtime dependency upgrades:

- `google.golang.org/grpc` `v1.76.0` -> `v1.83.2`
- `google.golang.org/protobuf` `v1.36.10` -> `v1.36.12`
- `github.com/spf13/cobra` `v1.10.1` -> `v1.10.2`
- `github.com/stretchr/testify` `v1.11.1` -> `v1.12.1`
- `go.uber.org/zap` `v1.27.0` -> `v1.28.0`
- migrated YAML import path:
  - removed `gopkg.in/yaml.v3`
  - added `go.yaml.in/yaml/v3 v3.0.5`

## Removed dependencies

No runtime module removals were finalized in this pass:

- `github.com/doitdistributed/go-update` is still used for executable replacement semantics.
- `github.com/mitchellh/go-ps` is still used for process management.

Release version scripts are retained and used by CI/CD:

- `scripts/semver_next.sh`
- `scripts/semver_next.ps1`

## gRPC changes

- `api/v1/alarm.proto` `go_package` fixed to `github.com/oshokin/alarm-button/internal/pb/v1;alarm`.
- Added server unary logging interceptor (`method`, `status`, `duration`).
- Added standard gRPC health service registration (`grpc_health_v1`).
- Added bounded graceful shutdown fallback (`GracefulStop` -> timeout -> `Stop`).
- Reworked server lifecycle around serve error channel to avoid cancellation path hangs.
- Added transport security policy:
  - non-loopback listener without TLS is rejected unless explicitly allowed.
- Client dialing API refactored:
  - removed fake `Dial(context.Context, ...)`
  - added `NewClient(address, cfg, opts...)`
  - TLS credentials support + insecure policy checks.

## Persistence fixes

- Added `internal/fsutil.WriteFileAtomic` and tests.
- Config and state persistence now use atomic write helper.
- Repository now honors `context.Context` cancellation in `Load`/`Save`.
- Server state commit ordering fixed:
  - persist to repository first
  - commit in-memory state only after successful disk write.
- Added regression test:
  - memory state is not committed on persistence failure.

## Updater security changes

- Updater architecture replaced with signed desired-state flow:
  - update lock file (`alarm-button-update.lock`) with PID payload
  - signed manifest (`alarm-button-version.yaml` + `.sig`)
  - role-local allowlist (`RoleSpec`) for binaries
  - fixed local config destination (`alarm-button-settings.yaml`)
  - size + SHA-512 verification for artifacts
  - bounded manifest download size
  - preflight before process stop
  - transactional apply with backup/rollback
  - persisted local updater state (`alarm-button-update-state.json`)
- Added role-aware centralized config metadata:
  - independent config revision/hash tracking
  - config drift repair semantics
  - config rollback rejection by default
- Added updater regression tests:
  - signature verification (valid/invalid/tampered)
  - manifest validation invariants
  - config-only update integration flow.

## Packager changes

- `alarm-packager` redesigned as pure packaging/signing tool:
  - no server reachability dependency
  - no operational config mutation
  - explicit `--input-dir` and `--output-dir`
  - Ed25519 PKCS8 private-key signing
  - manifest generation with platform/role config metadata
  - config YAML semantic validation before publication
- Added packager integration test for signed output generation.

## CI/CD changes

- CI workflow now includes explicit quality gates:
  - `lint`
  - `module-integrity` (`go mod verify`, `go mod tidy -diff`, `go vet`)
  - `generated-code` (protobuf regeneration + git diff check)
  - `security` (`govulncheck`)
  - `release-check` (`goreleaser check` + snapshot release)
  - `test-go-versions` (ubuntu/windows/macos matrix)
  - `test-build` (ubuntu/windows/macos matrix)
- Release workflow keeps semantic version scripts (`scripts/semver_next.sh`, `scripts/semver_next.ps1`)
  and creates tags/releases through `goreleaser/goreleaser-action@v7`.
- GitHub Actions references use major version tags (`@v6`, `@v7`, `@v9`)
  for `checkout`, `setup-go`, `golangci-lint-action`, and `goreleaser-action`.
- GoReleaser config modernized:
  - removed `go mod tidy` hook
  - removed `BuildTime` ldflag
  - added `-trimpath`
  - explicit `checksum` section (`sha256`)
  - release preflight checks.

## Documentation changes

- `README.md` was restored to the original tiger-allegory style and updated in place.
- Task commands and examples in README were synchronized with the current `taskfile.yaml`
  (`install-tools`, `generate-protobuf`, `lint-fix`, `test-race`, `install-githooks`, etc.).
- Packager usage examples were updated to the current flag-based CLI.

## Test changes

- Integration test flakiness reduced:
  - removed free-port close/rebind race
  - listener injected directly into server run path
  - readiness uses eventual RPC checks, not startup sleeps.
- Added new unit/integration coverage for:
  - atomic file writes
  - server transport policy
  - updater signature/manifest/config update logic
  - semantic version bump logic via scripts.
- Added Windows updater coverage:
  - deferred self-replacement acceptance scenario (`updater_windows_e2e_test.go`)
  - rollback regression that forbids scheduling self-replace helper on failed readiness.

## Security validation

Executed:

- `task lint` (PASS)
- `go test -count=1 ./...` (PASS)
- `go test -race -count=1 ./...` (PASS)
- `go vet ./...` (PASS)
- `go build ./...` (PASS)
- `go mod download` (PASS)
- `go mod verify` (PASS)
- `go mod tidy -diff` (PASS)
- `go generate ./...` (PASS)
- `goreleaser check` (PASS)
- `goreleaser release --snapshot --clean` (PASS)
- `go tool govulncheck ./...` (PASS)
- `task security` (PASS)

## Cross-platform validation

Local verification:

- `goreleaser release --snapshot --clean` built:
  - linux: `amd64`, `arm64`
  - windows: `amd64`, `arm64`
  - darwin: `amd64`, `arm64`

CI runtime matrix configured for:

- `ubuntu-latest`
- `windows-latest`
- `macos-latest`

## Quantitative diff

Repository snapshot metrics:

- Go files: 102
- Go LOC: 9648
- test files: 27
- direct dependencies: 8 -> 9
- indirect dependencies: 10 -> 13
- lint strategy: `linters.default: all` with a curated `disable` list plus strict per-linter settings
- CI jobs (build-and-test workflow): 3 -> 7
- tests (`Test*` + `Fuzz*` functions): 73 (`Fuzz*`: 0)

## Remaining limitations

- Windows self-update coverage now includes both locked-executable replacement and an acceptance flow,
  but the acceptance harness invokes `updater.Run()` from a test helper binary rather than launching
  the installed legacy `alarm-updater.exe` itself as the updater process.
- `WriteFileAtomic` still relies on `os.Rename`; Linux/macOS semantics are strong, while Windows does
  not guarantee the same atomic-replacement behavior under crash/power-loss timing edges.
