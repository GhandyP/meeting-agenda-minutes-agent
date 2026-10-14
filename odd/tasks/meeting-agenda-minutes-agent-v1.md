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
| T1 | Bootstrap: isolated `git init -b main`, MIT LICENSE, .gitignore, public GitHub repo, push | S11, S12 | inline | — |
| T2 | Own Go module `github.com/GhandyP/meeting-agenda-minutes-agent` on ADK Go v2.6.0 + genai; drop the unbuildable `internal/adkhelper` prototype import | S11, S12 | inline | — |
| T3 | Contract + deterministic validation (`internal/meeting`): input/output types, duration totals, mode-field exclusivity, unknown owner/date rules, transcript evidence | S2, S6, S7 | inline | — |
| T4 | Fixtures and edge cases (`testdata/`, loader): duration overflow, repeated topics, missing owners, missing timestamps, empty transcript, agenda vs minutes selection | S4, S8 | inline | — |
| T5 | Approval gate + dry-run adapters (`internal/approval`, `internal/calendar`, `internal/minutes`): proposal hash, approve/reject, ICS + minutes artifacts, refusal without a matching approval | S5, S9, S13 | inline | — |
| T6 | ADK layer (`internal/agent`): agenda and minutes `llmagent` branches with `OutputSchema`, deterministic Go mode router, meeting session; fake `model.LLM` so tests run without a key | S3, S7 | inline | — |
| T7 | Web app (`internal/server`, `web/`): JSON API, SSE progress, embedded UI, approval endpoint | S11, S12 | inline | — |
| T8 | Entrypoint (`main.go`): `serve` and headless `run` (`-input`, `-mode`, `-duration`, `-addr`, `-dry-run`, `-out`), help text | S10, S12 | inline | — |
| T9 | Delivery scaffolding: Makefile, Dockerfile, .dockerignore, CHANGELOG | S11 | inline | — |
| T10 | CI workflow (gofmt, vet, build, `go test -race`, docker build + keyless smoke) and README rewrite as the product README | S10, S11 | inline | — |
| T11 | Verification of the whole candidate: deterministic suite, keyless HTTP suite, live Gemini test skipped without a key, CI run | S6, S7, S8, S9 | verifier + CI | — |

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
