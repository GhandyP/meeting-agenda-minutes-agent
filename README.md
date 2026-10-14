# meeting-agenda-minutes-agent

[![CI](https://github.com/GhandyP/meeting-agenda-minutes-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/GhandyP/meeting-agenda-minutes-agent/actions/workflows/ci.yml)

A local, dry-run-only web app that turns meeting goals into a time-boxed agenda, or a transcript into evidence-backed minutes. The drafting runs on a **Google ADK (Go)** agent; every time box, owner, date and citation is then checked by pure-Go rules; and nothing is written until a human approves the exact proposal.

## What it does

Two modes behind one contract:

- **Agenda** — from `goals`, a `durationMinutes` and the `participants`, the agent drafts topics with an owner, a time box and the goal each topic serves. The rules refuse a plan whose time boxes do not fit the requested duration, that repeats a topic, that points at a goal that does not exist, or that invents an owner instead of naming one.
- **Minutes** — from `transcript` segments, the agent drafts decisions, actions and open questions, and every item must cite the transcript segment it came from. A missing or unknown citation is refused, because minutes that cannot show their evidence are not minutes.

The output shape is `AgendaOrMinutes`: `topics[]` for an agenda, `decisions[]`, `actions[]` and `openQuestions[]` for minutes. A proposal may never carry both, and the selected mode decides which fields are legal.

## The three layers

| Layer | Package | What it decides | Needs a model? |
| --- | --- | --- | --- |
| Contract and rules | `internal/meeting` | time boxes fit the requested duration, no duplicate topic, owners and dates explicit, evidence resolvable, mode exclusivity | no |
| Agent | `internal/agent` | drafts the proposal: one ADK `llmagent` branch per mode, chosen deterministically in Go before the model is called | yes |
| Human gate | `internal/approval` | one decision bound to the request-response pair, refusing a stale or mismatched approval | no |
| Dry-run artifacts | `internal/calendar`, `internal/minutes` | a deterministic RFC 5545 ICS preview and an evidence-quoting Markdown document, written only against a matching approval | no |

The web app (`internal/server`, `web/`) and the CLI (`main.go`) only wire those layers together; they hold no rule of their own.

## Quickstart

Requirement: Go 1.26.6 or newer, and a Gemini API key for the drafting step.

```bash
export GOOGLE_API_KEY="your-key"
make run                # serves http://127.0.0.1:8080
```

Open <http://127.0.0.1:8080>, fill the form, press **Generate proposal**, read the deterministic report, then approve, reject, or write the dry-run artifacts. The UI is a single embedded page with no CDN, no build step and no framework.

Prefer the terminal? The examples under `examples/` are ready to run:

```bash
go run . run -input examples/meeting-agenda.json -out ./out
go run . run -input examples/meeting-minutes.json -out ./out
```

## Headless CLI

```text
meeting-agenda-minutes-agent serve [-addr 127.0.0.1:8080] [-out ./out] [-allow-remote] [-model gemini-flash-latest]
meeting-agenda-minutes-agent run -input <meeting.json> [-mode agenda|minutes] [-duration 60] [-out ./out] [-model gemini-flash-latest] [-dry-run]
```

`run` drafts one proposal, prints the deterministic report as JSON and stops at the pending proposal, printing its id. It never grants the approval itself: writing the artifacts is a human decision taken in the UI or through the API.

| Exit code | Meaning |
| --- | --- |
| `0` | success |
| `1` | operational error (missing key, model failure, unusable listener) |
| `2` | usage error (bad flags, unreadable or malformed input file, refused remote bind) |
| `3` | the proposal failed deterministic validation, so nothing was recorded for approval |

`-dry-run` defaults to `true` and `-dry-run=false` is refused with exit code `2`: v1 never sends an invitation and never publishes minutes. `serve` binds loopback by default because the server has no authentication; a wider bind needs the explicit `-allow-remote` flag.

## HTTP API

| Method | Route | Body | Answers |
| --- | --- | --- | --- |
| `POST` | `/api/v1/plans` | `MeetingPlanInput` | `200 {proposal, report}`, or `422` with the blocking report and no proposal |
| `GET` | `/api/v1/proposals/{id}` | — | `200 {proposal, report}`, with the report recomputed |
| `POST` | `/api/v1/proposals/{id}/approve` | `{"confirm":true\|false}` | `200` with the approved proposal, or `409 confirmation_required` |
| `POST` | `/api/v1/proposals/{id}/reject` | — | `200` with the rejected proposal |
| `POST` | `/api/v1/proposals/{id}/artifacts` | — | `200 {files, dryRun:true}`, or `409 not_approved` |
| `GET` | `/api/v1/proposals/{id}/events` | — | `text/event-stream` with the recorded progress trail |
| `GET` | `/` | — | the embedded UI |

```bash
curl -sS localhost:8080/api/v1/plans \
  -H 'content-type: application/json' \
  -d @examples/meeting-agenda.json | head -40
```

Every failure is JSON with a stable `code` and a human `detail`:

| Code | Status | When |
| --- | --- | --- |
| `invalid_json` | `400` | the body is not one JSON object, or carries an unknown field |
| `request_too_large` | `413` | the body exceeds 1 MiB |
| `method_not_allowed` | `405` | the route exists but not for that method |
| `validation_failed` | `422` | the deterministic rules reject the proposal |
| `not_found` | `404` | no proposal carries that id, or no API route matches |
| `confirmation_required` | `409` | a human still has to confirm the owners or deadlines the report flags |
| `already_decided` | `409` | the proposal was already approved or rejected |
| `not_approved` | `409` | an artifact was requested for a proposal nobody approved |
| `generation_failed` | `502` | the model call failed |
| `artifacts_failed` | `500` | the dry-run artifact could not be written |
| `internal_error` | `500` | anything else |

## Why the approval is a real gate

The gate hashes the **request and the proposal together** (`approval.Hash(in, out)`) and binds the human decision to that digest. Editing the goals after the approval, or swapping the proposal, invalidates it: `Authorize` returns `ErrHashMismatch` and the writer refuses before touching the filesystem. The artifact file name is sanitized to `[A-Za-z0-9._-]`, so a meeting id such as `../../etc/passwd` cannot escape the output directory. Reading the approval and writing the artifact are separate steps, and only `POST /api/v1/proposals/{id}/artifacts` writes anything at all.

## Development

```bash
make help          # list the targets
make check-fmt     # fail on an unformatted Go file
make vet
make build         # bin/meeting-agenda-minutes-agent
make test          # deterministic suite, no API key
make test-race     # the same suite under -race
make docker-build
make clean
```

| Command | Covers |
| --- | --- |
| `go test ./internal/meeting/...` | the rules, driven by the fixtures in `testdata/` |
| `go test ./internal/approval/... ./internal/calendar/... ./internal/minutes/...` | the pair-bound gate, the ICS and the minutes document, including the refusals |
| `go test ./internal/agent/...` | the two ADK branches on an injected fake `model.LLM`, plus one live Gemini test that **skips** without `GOOGLE_API_KEY` |
| `go test ./internal/server/...` | the whole HTTP surface with `httptest`: 422 before proposing, confirmation before approving, `not_approved` before writing, JSON error codes, SSE termination |
| `go test .` | the CLI: usage, exit codes, the `-dry-run=false` refusal, the missing-key error and a real `serve` start/shutdown |

Every test except the live Gemini one runs offline with no key, which is what CI enforces.

## Container

```bash
docker build -t meeting-agenda-minutes-agent:local .
docker run --rm -p 127.0.0.1:8080:8080 \
  -e GOOGLE_API_KEY="$GOOGLE_API_KEY" \
  meeting-agenda-minutes-agent:local
```

The image is a static binary on distroless, running as non-root with no shell. It publishes on `127.0.0.1` on purpose: the container binds `0.0.0.0` with `-allow-remote` because that is the only address reachable inside a container network, and the host port is what keeps it private.

## Layout

```text
.
├── main.go                     # serve + headless run, exit codes, help
├── internal/
│   ├── meeting/                # contract, rules, fixtures loader
│   ├── agent/                  # ADK agenda and minutes branches
│   ├── approval/               # pair-bound gate and the shared name sanitizer
│   ├── calendar/               # deterministic dry-run ICS
│   ├── minutes/                # deterministic dry-run Markdown
│   └── server/                 # JSON API, SSE trail, single write path
├── web/                        # embedded UI (HTML, CSS, JS)
├── testdata/                   # validation fixtures
├── examples/                   # sample MeetingPlanInput files
└── odd/tasks/                  # the task document this was built from
```

## Limitations

- Text only: no audio capture, no speech-to-text, no Google Calendar OAuth client, no invitation that leaves dry run, no publication of minutes, no participant directory and no timezone database (timestamps are validated as supplied strings).
- No authentication: the server is a single-operator local tool and binds loopback unless you say otherwise.
- Generation is synchronous, so `GET /api/v1/proposals/{id}/events` replays the recorded progress trail of an existing proposal instead of streaming a live job. It is never required for the generate path to work.
- The live Gemini test only runs when you provide a key; without one it is reported as skipped, never as passing.

## License

MIT — see [LICENSE](LICENSE).
