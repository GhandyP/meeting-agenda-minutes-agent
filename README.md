# 17 — Meeting agent: agenda and minutes

## Goal

Produce a meeting agenda or minutes from supplied goals or transcript text, keeping time boxes, decisions, owners, and follow-ups explicit.

## Run the current prototype

From this case directory:

```bash
export GOOGLE_API_KEY="your-key"
go run .
```

`internal/adkhelper.Run` uses ADK Go v2.2.0, Gemini, and `gemini-flash-latest`; set `ADK_MODEL` to override the model.

Sample prompt:

```text
Turn these meeting goals into an agenda with time-boxed topics.
```

## What exists now

- `main.go (exists)` invokes one focused Gemini ADK agent over user-supplied goals or notes.
- The output is free-form; meeting mode, duration, topic owners, decisions, and action dates are not typed.
- This is intentionally text-only: it does not read a calendar, transcribe audio, create an invite, or publish minutes.

## Missing elements

- **Contract:** Define `MeetingPlanInput{mode, goals, duration, transcript?}` -> `AgendaOrMinutes{topics[], decisions[], actions[], openQuestions[]}`.
- **ADK workflow:** Add agenda and minutes instructions as separate workflow branches, a meeting session, and schemas; a future `create_event` tool must be approval-gated.
- **Domain boundary:** Add calendar, STT/Whisper, timezone, participant, and minutes-repository adapters.
- **Fixtures and validation:** Test duration totals, repeated topics, missing owners, transcript timestamps, and agenda/minutes mode selection.
- **Safety:** Keep invitations and published minutes in dry-run mode; a human must confirm participants, owners, and deadlines.

## Target structure

```text
17-meeting-agenda-minutes-agent/
├── main.go (exists)
├── README.md (exists)
├── workflow.go (next)
├── schema.go (next)
├── adapters/calendar.go (next)
├── testdata/meeting.json (next)
└── main_test.go (next)
```

## Implementation order

1. Define agenda, minutes, topic, action, and decision schemas.
2. Add mode routing and a session keyed by meeting ID.
3. Implement time-box and transcript evidence validation.
4. Test duration overflow, missing owners, and mode-specific outputs.
5. Add calendar and publication tools behind separate approvals.

## Done when

- [ ] Agenda time boxes fit the requested duration and minutes retain evidence.
- [ ] The selected mode produces only its allowed fields.
- [ ] Fixtures cover missing owners, timestamps, and repeated topics.
- [ ] Calendar changes and publication require human approval.

## Next improvement

Split agenda and minutes into typed workflow branches before connecting calendar or transcription adapters.
