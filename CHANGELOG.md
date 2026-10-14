# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

## [Unreleased]

### Added

- Typed meeting contract and deterministic validation: mode exclusivity, time-box totals against the requested duration, duplicate topics, goal indexes, owner and date confirmation, and transcript evidence resolution (`internal/meeting`).
- Fixture corpus under `testdata/` covering duration overflow, repeated topics, missing owners, a citation without a timestamp, an empty transcript and mode selection.
- Google ADK (Go) agent layer: separate agenda and minutes `llmagent` branches sharing one output schema, with the mode chosen deterministically in Go before the model is called (`internal/agent`).
- Human approval gate bound to the request-response pair, plus deterministic dry-run artifacts: an RFC 5545 ICS preview for agendas and an evidence-quoting Markdown document for minutes. Nothing is written without an approval that matches the exact pair (`internal/approval`, `internal/calendar`, `internal/minutes`).
- Local web app: versioned JSON API, keyless SSE progress trail, embedded UI and a single approval-gated write path (`internal/server`, `web/`).
- `serve` and headless `run` entrypoints with explicit exit codes and a `-dry-run=false` refusal (`main.go`).
- Makefile, container image and CI workflow covering formatting, vet, build, the race-enabled suite and a keyless container smoke test.

### Known limitations

- Text only: no audio, no speech-to-text, no calendar OAuth client, no invitation that leaves dry run, and no publication of minutes.
- The server has no authentication and binds loopback by default; `-allow-remote` is for container or lab use only.
- The SSE endpoint replays the recorded progress trail of a proposal rather than streaming a live job, because generation runs synchronously.
- The live Gemini test is skipped unless `GOOGLE_API_KEY` is set; everything else runs offline against a fake `model.LLM`.
