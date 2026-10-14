# meeting-agenda-minutes-agent — v1 Go web app

Public Go web app: a Google ADK (Go v2) agent turns supplied meeting goals into a time-boxed agenda
or a transcript into evidence-bearing minutes, and pure-Go rules validate every time box, owner, date
and citation before anything is created or published. The calendar event and the published minutes
stay in dry run and require a human approval token bound to that exact proposal.
v1 is text-only: no audio, no STT, no OAuth, no calendar write.

Repo: `github.com/GhandyP/meeting-agenda-minutes-agent` (public, MIT). Work lands on `main`, one work-unit commit per task.

## Specs

S1 — case-17 README "Goal" (verbatim): `Produce a meeting agenda or minutes from supplied goals or transcript text, keeping time boxes, decisions, owners, and follow-ups explicit.`

S2 — case-17 README "Missing elements → Contract" (verbatim): ``Define `MeetingPlanInput{mode, goals, duration, transcript?}` -> `AgendaOrMinutes{topics[], decisions[], actions[], openQuestions[]}`.``

S3 — case-17 README "Missing elements → ADK workflow" (verbatim): ``Add agenda and minutes instructions as separate workflow branches, a meeting session, and schemas; a future `create_event` tool must be approval-gated.``

S4 — case-17 README "Missing elements → Fixtures and validation" (verbatim): `Test duration totals, repeated topics, missing owners, transcript timestamps, and agenda/minutes mode selection.`

S5 — case-17 README "Missing elements → Safety" (verbatim): `Keep invitations and published minutes in dry-run mode; a human must confirm participants, owners, and deadlines.`

S6 — case-17 README "Done when" (verbatim): `Agenda time boxes fit the requested duration and minutes retain evidence.`

S7 — case-17 README "Done when" (verbatim): `The selected mode produces only its allowed fields.`

S8 — case-17 README "Done when" (verbatim): `Fixtures cover missing owners, timestamps, and repeated topics.`

S9 — case-17 README "Done when" (verbatim): `Calendar changes and publication require human approval.`

S10 — case-17 README "What exists now" (verbatim): `This is intentionally text-only: it does not read a calendar, transcribe audio, create an invite, or publish minutes.`

S11 — user (verbatim): `1 el reconocimiento y explorar , crear el repo de github con git y gh cli , crear un plan odd para crear desde cero en este repo el proyecto completo y lista para mi portafolio en github , con go google adk`

S12 — user decisions (verbatim labels): repo/module `meeting-agenda-minutes-agent`; extra scope `(custom) que sea web app de go`; calendar `Dry-run local + gate de aprobación`; delivery `Sí: público, MIT, directo a main`.

S13 — case-17 README "Missing elements → Domain boundary" (verbatim): `Add calendar, STT/Whisper, timezone, participant, and minutes-repository adapters.`

Non-goals (v1, from S5/S10/S13): no audio or STT adapter, no Google Calendar OAuth client, no calendar invite that leaves dry run, no minutes publication, no participant directory service, no timezone database dependency (timestamps are validated as supplied strings, not re-derived).

## Tasks

| ID | Task | Specs | Route | Commit |
| --- | --- | --- | --- | --- |
| T1 | Bootstrap: isolated `git init -b main`, MIT LICENSE, .gitignore, public GitHub repo, push | S11, S12 | inline | af0ea17 |
| T2 | Own Go module `github.com/GhandyP/meeting-agenda-minutes-agent` on ADK Go v2.6.0 + genai; drop the unbuildable `internal/adkhelper` prototype import | S11, S12 | inline | 4d33476 |
| T3 | Contract + deterministic validation (`internal/meeting`): input/output types, duration totals, mode-field exclusivity, unknown owner/date rules, transcript evidence | S2, S6, S7 | inline | 4aae188 |
| T4 | Fixtures and edge cases (`testdata/`, loader): duration overflow, repeated topics, missing owners, missing timestamps, empty transcript, agenda vs minutes selection | S4, S8 | worker | fd114c2 |
| T5 | Approval gate + dry-run adapters (`internal/approval`, `internal/calendar`, `internal/minutes`): proposal hash, approve/reject, ICS + minutes artifacts, refusal without a matching approval | S5, S9, S13 | worker | c012afe |
| T6 | ADK layer (`internal/agent`): agenda and minutes `llmagent` branches with `OutputSchema`, deterministic Go mode router, meeting session; fake `model.LLM` so tests run without a key | S3, S7 | worker | d616d7f |
| T7 | Web app (`internal/server`, `web/`): JSON API, SSE progress, embedded UI, approval endpoint | S11, S12 | inline | ae8d275 |
| T8 | Entrypoint (`main.go`): `serve` and headless `run` (`-input`, `-mode`, `-duration`, `-addr`, `-dry-run`, `-out`), help text | S10, S12 | inline | 3f343c0 |
| T9 | Delivery scaffolding: Makefile, Dockerfile, .dockerignore, CHANGELOG | S11 | inline | b59c273 |
| T10 | CI workflow (gofmt, vet, build, `go test -race`, docker build + keyless smoke) and README rewrite as the product README | S10, S11 | inline | 1ec74dd |
| T11 | Verification of the whole candidate: deterministic suite, keyless HTTP suite, live Gemini test skipped without a key, CI run | S6, S7, S8, S9 | inline + CI | verification (L24) |

## Log

L1 — user (verbatim): `1 el reconocimiento y explorar , crear el repo de github con git y gh cli , crear un plan odd para crear desde cero en este repo el proyecto completo y lista para mi portafolio en github , con go google adk`

L2 — user decisions (verbatim labels): repo/module `meeting-agenda-minutes-agent`; extra scope `(custom) que sea web app de go`; calendar `Dry-run local + gate de aprobación`; delivery `Sí: público, MIT, directo a main`. The base layer (typed contract, deterministic Go validation, ADK agent, CLI, tests, CI, README) was accepted as given.

L3 — exploration evidence (2026-10-14): this directory had no local `.git`; `git rev-parse --show-toplevel` resolved to `/home/ghandy` (a repo with no commits and no remotes), so no Git operation may run from that boundary. `GhandyP/adk-go-enterprise-cookbook` does not exist on GitHub (`gh repo view` → `Could not resolve to a Repository`), so the `internal/adkhelper` import in the prototype is unbuildable and v1 must be self-contained. Redacted from this repository: the prototype's `README.md` and `main.go` are kept verbatim in T1 as the case specification and are replaced at T2/T10.

L4 — environment evidence: `go1.26.4`, `GOTOOLCHAIN=auto`, `GOMODCACHE=/home/ghandy/go/pkg/mod`, 111G free disk. `gh` authenticated as `GhandyP` (`repo`, `workflow` scopes). Git identity `GhandyP <GhandyP@users.noreply.github.com>`. In a scratch directory, `go get google.golang.org/adk/v2@latest` resolved `v2.6.0` and reported `requires go >= 1.26.6; switching to go1.26.9`, so the module raises the documented Go requirement above the README's original wording; the toolchain download works on this machine. The module cache already holds `adk/v2` `v2.2.0`, `v2.4.0`, `v2.6.0` and `genai` `v1.74.0`.

L5 — ADK Go v2.6.0 API evidence, read from the module cache (`.../google.golang.org/adk/v2@v2.6.0/`):
`model/llm.go` — `type LLM interface { Name() string; GenerateContent(ctx, *LLMRequest, stream bool) iter.Seq2[*LLMResponse, error] }`, with `LLMRequest{Model, Contents, Config, Tools}` and `LLMResponse{Content, Partial, TurnComplete, ...}`.
`agent/llmagent/llmagent.go` — `New(Config)` with `Config.Model` of type `model.LLM`, plus `InputSchema` and `OutputSchema` fields.
`runner/runner.go:212` — `NewInMemory(appName string, a agent.Agent) (*Runner, error)` with `AutoCreateSession: true` at `:219`; `:538` `func (r *Runner) Run(ctx context.Context, userID, sessionID string, msg *genai.Content, cfg agent.RunConfig, opts ...RunOption) iter.Seq2[*session.Event, error]`.
`session/session.go:144` — `Output any`; `:218` — `func (e *Event) IsFinalResponse() bool`.
`session/inmemory.go` — in-memory session service.
`agent/workflowagent/workflow.go`, `workflow/{base_node,agent_node,branch,config,dynamic_node}.go` — graph engine; `examples/workflow/hitl_simple/main.go` shows `workflow.NewEmittingFunctionNode[any, any](name, func(ctx agent.Context, _ any, emit func(*session.Event) error) (any, error), workflow.NodeConfig{})`, `workflow.NewRequestInputEvent(ctx, session.RequestInput{InterruptID, Message})`, `workflow.ErrNodeInterrupted`, and `workflowagent.New(workflowagent.Config{Name, Description, Edges: workflow.Chain(workflow.Start, ask, greet)})`.
`tool/toolconfirmation/tool_confirmation.go` — `const FunctionCallName = "adk_request_confirmation"`, `ToolConfirmation{Confirmed bool, Payload any}`, `OriginalCallFrom(*genai.FunctionCall) (*genai.FunctionCall, error)`.
`cmd/launcher` + `cmd/launcher/full` — `full.NewLauncher().Execute(ctx, *launcher.Config{AgentLoader: agent.NewSingleLoader(a)}, os.Args[1:])`, with `console` and `web api webui` (the ADK web UI is documented as development-only).
`model/gemini` — `NewModel(ctx, name, *genai.ClientConfig{APIKey})`; ADK Go quickstart uses `gemini-flash-latest`.

L6 — architecture decision (from S12 `que sea web app de go`): the product is an HTTP web app, not only a CLI. `internal/server` serves a versioned JSON API plus an SSE progress stream and an embedded UI (`go:embed`), and `main.go` keeps a headless `run` path so the whole pipeline stays testable in CI. The deterministic rules (duration, ownership, mode exclusivity, evidence, approval) live in pure Go packages with no ADK and no network, so the web app is verifiable without an API key.

L7 — testability decision (from L5): because `llmagent.Config.Model` accepts any `model.LLM`, tests inject a fake LLM that replays canned structured output. The full HTTP path (upload → validate → approve → dry-run artifacts) is therefore tested on CI with no `GOOGLE_API_KEY`; the only live test against Gemini is a single test that skips without a key, matching the precedent of `adk-meeting-summary`.

L8 — approval decision (from S5/S9): v1 approval is a pure-Go gate in `internal/approval` that binds a human approval token to a hash of the exact proposal, not the ADK `toolconfirmation` resume flow. Rationale: the gate must be provable in tests and must refuse a stale or mismatched approval, and the ADK confirmation flow in v2.6.0 is exercised by the launcher's console flow, not by our own HTTP API. ADK `toolconfirmation` stays available for the console/launcher surface and is documented as an optional layer, not as the enforcement point.

L9 — routing constraint (inherited incident, `adk-meeting-summary` L8/L19, reproduced with the same Git boundary): this session started with `/home/ghandy` as its Git root; creating an isolated repository in a subdirectory invalidates that registered identity, and subagent worktree registration can fail with `Session Git identity changed; cannot register a worktree`. Plan: exploration and planning run before `git init`; every task is routed `inline`; T11 attempts the independent verifier once and reports it as unavailable if that host error returns, with the CI run reported as execution evidence instead. No line here claims an independent review that did not happen.

L10 — remote-credential decision (from S12 `Sí: público, MIT, directo a main`): `gh repo create GhandyP/meeting-agenda-minutes-agent --public --source=. --remote=origin --push` after a local `git init -b main` with the MIT LICENSE. No force pushes; each task is one work-unit commit pushed to `main`, and the `Commit` column is filled in when the next work unit lands (as in `adk-meeting-summary`).

L11 — open item: the live Gemini round trip needs `GOOGLE_API_KEY`, which is not present in this environment. Until the user provides it, the ADK wiring is compiled, vetted and exercised only through the fake `model.LLM`; the live test is reported as skipped, never as passing.

L12 — seam freeze (2026-10-14), binding for T4–T8. Every task implements these signatures exactly; a task that needs a different seam stops and reports instead of inventing one. `internal/meeting` (T3, already implemented) is the only shared type source: `Validate(in MeetingPlanInput, out AgendaOrMinutes) Report` plus the `Code*`/`Severity*` constants. `internal/approval` must not import `internal/agent`, and `internal/calendar`/`internal/minutes` may import `internal/approval` and `internal/meeting` only.

```go
// internal/agent (T6) — no package-level model construction; tests inject model.LLM
package agent

type Config struct {
	Model     model.LLM // required
	ModelName string    // optional label; defaults to Model.Name()
}

type Generator interface {
	Generate(ctx context.Context, in meeting.MeetingPlanInput) (meeting.AgendaOrMinutes, error)
}

func New(cfg Config) (Generator, error)

type GeminiConfig struct {
	APIKey    string
	ModelName string // default "gemini-flash-latest"
}

func NewGeminiModel(ctx context.Context, cfg GeminiConfig) (model.LLM, error)
```

```go
// internal/approval (T5)
package approval

type State string

const (
	StatePending  State = "pending"
	StateApproved State = "approved"
	StateRejected State = "rejected"
)

type Proposal struct {
	ID          string                 `json:"id"`
	MeetingID   string                 `json:"meetingId"`
	Mode        meeting.Mode           `json:"mode"`
	PayloadHash string                 `json:"payloadHash"`
	Payload     meeting.AgendaOrMinutes `json:"payload"`
	State       State                  `json:"state"`
	CreatedAt   time.Time              `json:"createdAt"`
}

var ErrNotFound error    // unknown proposal id
var ErrNotApproved error  // proposal exists but is not approved
var ErrHashMismatch error // approval is bound to a different payload

func Hash(p meeting.AgendaOrMinutes) (string, error) // canonical JSON of p, sha256, lowercase hex

type Gate interface {
	Propose(p meeting.AgendaOrMinutes) (Proposal, error)
	Approve(id string) (Proposal, error)
	Reject(id string) (Proposal, error)
	Get(id string) (Proposal, error)
	Authorize(id string, p meeting.AgendaOrMinutes) error // nil only when approved AND hash(p) matches the bound hash
}

func NewGate() Gate // in-memory, concurrency-safe
```

```go
// internal/calendar (T5) — deterministic text, never a network call
package calendar

func ICS(p meeting.AgendaOrMinutes) ([]byte, error)
func WriteDryRun(dir string, g approval.Gate, token string, p meeting.AgendaOrMinutes) (string, error)
```

```go
// internal/minutes (T5) — deterministic text, never a publish call
package minutes

func Document(p meeting.AgendaOrMinutes) ([]byte, error)
func WriteDryRun(dir string, g approval.Gate, token string, p meeting.AgendaOrMinutes) (string, error)
```

`calendar.WriteDryRun` and `minutes.WriteDryRun` call `g.Authorize(token, p)` first and return that error unchanged when it is non-nil: no file is created without an approval bound to that exact payload. Both return the written path.

```go
// internal/meeting fixtures (T4)
package meeting

type Case struct {
	Name             string           `json:"name"`
	Spec             string           `json:"spec"`
	Input            MeetingPlanInput `json:"input"`
	Output           AgendaOrMinutes  `json:"output"`
	WantProblemCodes []string         `json:"wantProblemCodes"`
}

func LoadCases(dir string) ([]Case, error) // *.json in dir, filename order
```

L13 — session evidence (2026-10-14): `gh repo view GhandyP/meeting-agenda-minutes-agent` reports `PUBLIC`, default branch `main`; the local `main` was one commit ahead of `origin/main` with `internal/` untracked, so T3 existed on disk but uncommitted. `go build ./...`, `go vet ./...` and `go test ./... -count=1` reported `ok internal/meeting`; `gofmt -l` flagged `internal/meeting/validate_test.go`, which was formatted before the T3 commit `4aae188`. Local Pi tool state (`.codegraph/`) was added to `.gitignore` so the public repository carries no machine-local artifacts. `GOOGLE_API_KEY` is absent, so L11 stands; T2 (`4d33476`) and T3 (`4aae188`) were pushed to `origin/main`.

L14 — seam freeze for T7/T8 (2026-10-14), binding once T5 and T6 land. T7 owns the HTTP surface and never reimplements validation, decoding or the approval gate; it calls `agent.Generator.Generate`, `meeting.Validate` and `approval.Gate`. T8 owns process wiring only.

```go
// internal/server (T7)
package server

type Config struct {
	Generator agent.Generator
	Gate      approval.Gate
	OutDir    string       // dry-run artifact directory
	Logger    *slog.Logger // optional; defaults to a discard logger
}

type Server struct{ /* unexported */ }

func New(cfg Config) (*Server, error)
func (s *Server) Handler() http.Handler // every route below, plus the embedded UI
```

Routes: `POST /api/v1/plans` (body is `meeting.MeetingPlanInput`) generates, validates and proposes; `GET /api/v1/proposals/{id}`; `POST /api/v1/proposals/{id}/approve` and `.../reject`; `POST /api/v1/proposals/{id}/artifacts` writes the dry-run artifacts; `GET /api/v1/proposals/{id}/events` streams generation progress as SSE; `GET /` serves the embedded UI from `web/` via `go:embed`.

Required HTTP behavior, each point covered by a `httptest` case with a fake generator (no ADK, no network, no API key):
- `POST /api/v1/plans` with a payload whose `meeting.Validate` report has errors answers `422` with the report and creates no proposal and no file.
- A valid payload answers `200` with the proposal and the report; the proposal is retrievable at `GET /api/v1/proposals/{id}`.
- `POST .../approve` with a body that omits the confirmation flag while the report carries warnings answers `409` and leaves the proposal pending; with the flag it answers `200` and the proposal is approved.
- `POST .../artifacts` before approval answers `409` and writes no file; after approval it writes the artifacts and answers `200` with the written paths and `dryRun: true`.
- An unknown proposal id answers `404` on every proposal route. Every 4xx body is JSON with a stable `code` and `detail` field.
- `GET .../events` answers `text/event-stream`, emits at least one `data:` event and terminates; it must not be required for the generate path to work.
- No route can create a calendar invite or publish minutes: the only write path is `.../artifacts`, and only for an approved proposal.

T8 command contract (`main.go`): `serve` and `run` subcommands. `serve -addr -out` starts the HTTP server; `run -input -mode -duration -out` is headless: it reads a `meeting.MeetingPlanInput` JSON file, generates, validates, prints the report as JSON to stdout and stops at the proposal, printing its id and stating that the dry-run artifacts require the approval step. `-dry-run` defaults to true and `-dry-run=false` is refused with a clear error and no side effect, because v1 never leaves dry run. Exit codes: 0 success, 1 operational error, 2 usage error, 3 the report has errors. Missing `GOOGLE_API_KEY` is an operational error with a message naming the variable; `-h`/`--help` documents both subcommands and every flag.

L15 — routing decision (2026-10-14): T4, T5 and T6 were delegated to three `gentle-ai-worker` tasks in one batch on disjoint edit surfaces (`testdata/**` + `internal/meeting/fixtures*`; `internal/approval/**`, `internal/calendar/**`, `internal/minutes/**`; `internal/agent/**`), with L12 as the binding seam and an explicit ban on `go mod tidy`, on full-module builds (other packages are mid-flight), and on git write commands. T9/T10 land only after T8, so the public `main` never carries a CI or Docker job that references a binary that does not exist yet.

L16 — seam correction (2026-10-14), supersedes the `internal/approval`, `internal/calendar` and `internal/minutes` blocks of L12. The T5 worker stopped before editing because those signatures could not express the required behavior: `calendar.ICS(AgendaOrMinutes)` has no requested duration — `AgendaOrMinutes` carries only the time boxes, and summing them is not the requested duration (the fixture family requests 60 minutes and boxes 55) — and `minutes.Document(AgendaOrMinutes)` has only transcript segment ids, not the transcript text, so it could not retain the evidence S4/S8 require. Both facts live in `MeetingPlanInput`.

The approval therefore binds the request-response pair rather than the response alone: a human approves "this request produced this proposal", and changing the request invalidates the approval. No task needed edit surfaces beyond `internal/approval`, `internal/calendar` and `internal/minutes`: `internal/meeting` is a read-only dependency.

```go
// internal/approval (T5, supersedes L12)
package approval

type State string

const (
	StatePending  State = "pending"
	StateApproved State = "approved"
	StateRejected State = "rejected"
)

type Proposal struct {
	ID          string                        `json:"id"`
	MeetingID   string                        `json:"meetingId"`
	Mode        meeting.Mode                  `json:"mode"`
	Input       meeting.MeetingPlanInput      `json:"input"`
	Output      meeting.AgendaOrMinutes       `json:"output"`
	PayloadHash string                        `json:"payloadHash"`
	State       State                         `json:"state"`
	CreatedAt   time.Time                     `json:"createdAt"`
}

var ErrNotFound error       // unknown proposal id
var ErrNotApproved error     // proposal exists but is not approved
var ErrHashMismatch error    // approval is bound to a different request-response pair
var ErrAlreadyDecided error  // approve or reject on a decided proposal
var ErrModeMismatch error    // input mode is unknown or output mode differs from it

func Hash(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (string, error) // canonical JSON of the pair, sha256, lowercase hex

type Gate interface {
	Propose(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (Proposal, error)
	Approve(id string) (Proposal, error)
	Reject(id string) (Proposal, error)
	Get(id string) (Proposal, error)
	Authorize(id string, in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) error
}

func NewGate() Gate // in-memory, safe for concurrent use
```

```go
// internal/calendar (T5, supersedes L12) — deterministic text, never a network call
package calendar

func ICS(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) ([]byte, error)
func WriteDryRun(dir string, g approval.Gate, token string, in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (string, error)

// internal/minutes (T5, supersedes L12) — deterministic text, never a publish call
package minutes

func Document(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) ([]byte, error)
func WriteDryRun(dir string, g approval.Gate, token string, in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (string, error)
```

T5 behavior additions, all testable: `Propose` generates a stable hex `ID` from `crypto/rand`, mirrors `in.MeetingID` into `MeetingID` and `in.Mode` into `Mode`, and returns `ErrModeMismatch` when `in.Mode` is neither agenda nor minutes or `out.Mode` differs from it. `calendar.ICS` takes `DURATION:PT<N>M` from `in.DurationMinutes`, `SUMMARY` from `out.Title` falling back to `in.Title`, and the time boxes from `out.Topics`; it errors when the mode is not agenda, when the duration is not positive, or when there is no topic. `minutes.Document` resolves every evidence id against `in.Transcript` and renders the segment speaker, its timestamp when the supplied string is non-empty, and the segment text as the retained quote; an evidence id that is not a segment of `in.Transcript` is an error naming that id, because a publication must never invent evidence.

L17 — L14 clarification (2026-10-14): the `POST /api/v1/proposals/{id}/artifacts` route takes no request body. It renders from the stored proposal's own `Input` and `Output`, so a caller can never supply bytes that differ from what the human approved.

L18 — T4 and T6 evidence (2026-10-14). T4 (`fd114c2`) was verified by the parent before committing: `gofmt -l internal/meeting/` clean, `go vet` and `go test ./internal/meeting/... -count=1` ok, and the eight fixtures read by hand to confirm each expected code is the rule it claims. T6 (`d616d7f`) could not be verified by its worker: focused commands failed on missing `go.sum` entries for `github.com/google/uuid`, `rsc.io/omap`, `rsc.io/ordered`, `github.com/google/jsonschema-go` and `github.com/google/safehtml`, which the ADK runner imports; the worker correctly refused to edit `go.mod`/`go.sum` and reported the blocker with no GREEN. The parent resolved it with `GOFLAGS=-mod=mod go build ./internal/agent/...`, which added 12 `go.sum` entries and promoted `google.golang.org/adk/v2` and `genai` to direct requires, then observed the missing GREEN: `gofmt -l internal/agent/` clean, `go vet ./internal/agent/...` clean, `go test ./internal/agent/... -count=1 -v` → six tests PASS and `TestGenerateLiveGemini` SKIP, which is reported as skipped, never as passing. One incidental symptom worth remembering: the T4 worker's first post-implementation test run timed out at 120 seconds while the ADK imports were compiling concurrently, and the immediate rerun passed.

L19 — T5 evidence and delivered API (2026-10-14, `c012afe`). The T5 worker reported one honest deviation: strict test-first ordering held for the artifact packages but behaviors 3–5 of the gate were implemented before their individual tests were added, so no RED was observed for them; the tests exist and pass. The parent verified the delivered work before committing: `gofmt -l` clean over the three packages, `go vet` clean, and `timeout 110 go test ./internal/meeting/... ./internal/approval/... ./internal/calendar/... ./internal/minutes/... -count=1 -race` → `ok` for all four packages. Delivered API beyond L16, binding for T7: `approval.ArtifactFileName(meetingID, ext string) (string, error)` is the single sanitizer both writers use, where `ext` is bare (`"ics"`, `"md"`) and the function adds the dot; `Hash` re-encodes both sides into explicit canonical structs rather than marshalling the contract types, so a future field addition cannot silently change an existing digest; `calendar.ICS` derives `UID` from that digest and lists topics as `X-TOPIC` lines, so the artifact carries no clock-derived value; `minutes.ErrEvidenceNotFound` wraps the id that was cited but absent.

L20 — T7/T8 implementation decisions (2026-10-14), binding for T7 and T8:
- Artifacts are per mode: an agenda proposal writes the ICS, a minutes proposal writes the Markdown document, and the response body is `{"files":[<path>],"dryRun":true}`. Nothing else in the server writes to disk.
- The validation report is never stored; `GET /api/v1/proposals/{id}`, the approve route and the artifacts route recompute `meeting.Validate(proposal.Input, proposal.Output)`, which is deterministic by S6/S7.
- `POST /api/v1/proposals/{id}/approve` accepts `{"confirm":true}`. It answers `409` while the recomputed report requires confirmation (S5: a human confirms owners and deadlines) and `200` otherwise; `reject` takes no body.
- v1 has no job queue: `POST /api/v1/plans` is synchronous and the server records that proposal's progress events (received, generated, validated, proposed, or failed) as they happen. `GET /api/v1/proposals/{id}/events` replays those recorded events as SSE and closes. It is a snapshot trail, not a live stream, and it is never required for the generate path to work; the README must say so.
- The UI lives at the repository root in `web/` with its own `package web` that embeds `*.html`, `*.js` and `*.css` via `go:embed` and exposes an `http.Handler`; `internal/server` mounts it at `/`. Go cannot embed files outside the embedding package's directory, which is why `web/` carries a Go file.
- Every 4xx body is JSON with a stable `code` and a human `detail`: `invalid_json`, `invalid_request`, `request_too_large`, `validation_failed`, `not_found`, `confirmation_required`, `not_approved`, `artifacts_failed`, `internal_error`. Unknown ids answer `404` on every proposal route; wrong methods answer `405`.
- The server has no authentication and writes files: `-addr` defaults to `127.0.0.1:8080`, and T8 refuses a non-loopback address unless `-allow-remote` is passed explicitly. The README states this is a local dry-run tool.

L21 — environment note (2026-10-14): several other Pi sessions on this machine were running `go mod tidy`, `go build` and `go test` in their own repositories at the same time, and a whole-module `go test ./...` stalled twice in `futex_do_wait` on the shared Go build cache — once as a 300-second synchronous timeout and once as a background job that made no progress for two minutes and was stopped. Focused per-package runs complete in about a second. Verification therefore uses the four-package `-race` command above, and a whole-module run is only trusted when it actually produces output.

L22 — worker availability (2026-10-14): the subagent path stopped working after six successful delegated tasks. The `subagent_continue` call for the corrected T5 stalled at `starting` with zero turns and was reported as a timeout, and a fresh background `gentle-ai-worker` for T7 stalled the same way. Per the harness rule that a timeout is a failure and an unavailable subagent is reported rather than retried, T7 was implemented inline instead of launching a third attempt. Only the parent could observe the process state: the stalls were reported by the harness, never papered over, and no task was credited on the strength of a stalled run.

L23 — T7–T10 evidence (2026-10-14). T7 (`ae8d275`) was written test-first: `go test ./internal/server/...` first failed with `no non-test Go files`, then ten `httptest` cases passed over `server.Handler()` with a fake generator and a real gate, including the 422 before proposing, the confirmation refusal, the `not_approved` refusal, the JSON error codes, the SSE termination and the single write path. T8 (`3f343c0`) was written test-first the same way: `undefined: run/newGenerator/usage` was observed before `main.go` existed, and eleven root tests then passed, including a real `serve` start over a listener plus a shutdown on context cancellation. Two failures during that work were genuine and were fixed rather than loosened: the warning message said `require` where the test expected `requires`, and the serve test blocked the server because an `io.Pipe` hands over one write at a time, so the test now drains it in the background. T9 (`b59c273`) and T10 (`1ec74dd`) are configuration and documentation, so there was no failing test to write; `make check-fmt`, `make vet`, `make build`, `make help`, the YAML parse of the workflow and the new `examples/` loader test are the evidence instead. The first container build failed for a network reason, not a code reason: `go mod download` inside the build stage hit `TLS handshake timeout` against `proxy.golang.org`. That failure is recorded here as a local verification gap, the container image is retried over the host network, and the CI container job is the independent check.

L24 — T11 evidence (2026-10-14). The candidate is complete against S6–S9 and its verification is the following, all observed rather than assumed. CI run `38503020864` for `a06b438` completed `success` with both jobs green: *Format, vet, build, race test* ran the gofmt check, `go vet ./...`, `go build ./...` and `go test ./... -count=1 -race` with `GOOGLE_API_KEY` empty, and the CI log records `--- SKIP: TestGenerateLiveGemini`, reported as skipped and never as passing; *Container image* built the image on the runner and ran both keyless smoke checks (`-h`, and `run -dry-run=false` with its expected exit code `2`), which is what verifies the Dockerfile, since the local default bridge network cannot reach `proxy.golang.org` from this machine. Locally the same day: the root package's eleven tests, the six internal packages, the four-package `-race` run, `make check-fmt`, `make vet`, `make build`, `make help` and the workflow YAML parse all pass. The first CI run for the previous commit shows as `cancelled` because this repository's own `cancel-in-progress` concurrency rule replaced it with the newer push; that is recorded here so a cancelled badge is never mistaken for a red build. Known gaps, stated rather than hidden: the live Gemini round trip needs `GOOGLE_API_KEY`, which this environment does not have, so that single test is skipped everywhere; and the ADK wiring is exercised offline through a fake `model.LLM`. The local container build then finished over the host network and its smoke checks matched the documented contract: `-h` answered `0` with the usage text, `run -dry-run=false` answered `2` with the refusal, and `run -input /nonexistent` answered `2` naming the unreadable file, before any model call.

