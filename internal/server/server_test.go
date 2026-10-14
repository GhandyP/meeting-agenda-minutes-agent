package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/approval"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/server"
)

const agendaJSON = `{"meetingId":"weekly-sync","mode":"agenda","title":"Weekly sync","durationMinutes":30,"goals":["Ship the release"],"participants":["Ana"]}`

const minutesJSON = `{"meetingId":"team-sync","mode":"minutes","title":"Team sync","participants":["Ana"],"transcript":[{"id":"s1","speaker":"Ana","timestamp":"00:00:10","text":"We will ship on Friday."}]}`

type fakeGenerator struct {
	mu     sync.Mutex
	output meeting.AgendaOrMinutes
	err    error
	calls  int
}

func (f *fakeGenerator) Generate(_ context.Context, _ meeting.MeetingPlanInput) (meeting.AgendaOrMinutes, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return meeting.AgendaOrMinutes{}, f.err
	}
	return f.output, nil
}

func (f *fakeGenerator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type proposalBody struct {
	Proposal approval.Proposal `json:"proposal"`
	Report   meeting.Report    `json:"report"`
}

type artifactsBody struct {
	Files  []string `json:"files"`
	DryRun bool     `json:"dryRun"`
}

type errorBody struct {
	Code   string          `json:"code"`
	Detail string          `json:"detail"`
	Report *meeting.Report `json:"report"`
}

func agendaOutput(owner string, minutes int) meeting.AgendaOrMinutes {
	return meeting.AgendaOrMinutes{
		Mode:  meeting.AgendaMode,
		Title: "Weekly sync",
		Topics: []meeting.Topic{
			{Title: "Ship the release", Owner: owner, Minutes: minutes, GoalIndex: 0},
		},
	}
}

func minutesOutput() meeting.AgendaOrMinutes {
	return meeting.AgendaOrMinutes{
		Mode:  meeting.MinutesMode,
		Title: "Team sync",
		Decisions: []meeting.Decision{
			{Statement: "Ship on Friday.", Evidence: []string{"s1"}},
		},
		Actions: []meeting.Action{
			{Task: "Update the checklist", Owner: "Ana", DueDate: "2026-10-22", Evidence: []string{"s1"}},
		},
	}
}

type harness struct {
	server    *server.Server
	generator *fakeGenerator
	gate      approval.Gate
	outDir    string
}

func newHarness(t *testing.T, output meeting.AgendaOrMinutes) *harness {
	t.Helper()
	generator := &fakeGenerator{output: output}
	gate := approval.NewGate()
	outDir := filepath.Join(t.TempDir(), "out")
	srv, err := server.New(server.Config{Generator: generator, Gate: gate, OutDir: outDir})
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	return &harness{server: srv, generator: generator, gate: gate, outDir: outDir}
}

func (h *harness) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	recorder := httptest.NewRecorder()
	h.server.Handler().ServeHTTP(recorder, httptest.NewRequest(method, path, reader))
	return recorder
}

func decode[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	return value
}

func assertNoArtifacts(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("read artifact directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("artifact directory holds %d entries, want none", len(entries))
	}
}

func TestPlansRejectsProposalThatFailsValidation(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 60))

	recorder := h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (body %s)", recorder.Code, recorder.Body.String())
	}
	body := decode[errorBody](t, recorder)
	if body.Code != "validation_failed" {
		t.Errorf("code = %q, want validation_failed", body.Code)
	}
	if body.Report == nil || !body.Report.HasErrors() {
		t.Fatalf("report = %+v, want a report carrying errors", body.Report)
	}
	if !hasProblem(body.Report, meeting.CodeDurationOverflow) {
		t.Errorf("report codes = %+v, want %s", body.Report.Problems, meeting.CodeDurationOverflow)
	}
	assertNoArtifacts(t, h.outDir)
}

func hasProblem(report *meeting.Report, code string) bool {
	for _, problem := range report.Problems {
		if problem.Code == code {
			return true
		}
	}
	return false
}

func TestPlansProposesValidInputAndServesIt(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 30))

	recorder := h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	created := decode[proposalBody](t, recorder)
	if created.Proposal.ID == "" {
		t.Fatal("proposal id is empty")
	}
	if created.Proposal.State != approval.StatePending {
		t.Errorf("state = %q, want pending", created.Proposal.State)
	}
	if created.Report.HasErrors() {
		t.Errorf("report = %+v, want no errors", created.Report)
	}
	if h.generator.callCount() != 1 {
		t.Errorf("generator calls = %d, want 1", h.generator.callCount())
	}

	fetched := h.do(t, http.MethodGet, "/api/v1/proposals/"+created.Proposal.ID, "")
	if fetched.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", fetched.Code, fetched.Body.String())
	}
	served := decode[proposalBody](t, fetched)
	if served.Proposal.ID != created.Proposal.ID {
		t.Errorf("served id = %q, want %q", served.Proposal.ID, created.Proposal.ID)
	}
	if served.Report.Mode != meeting.AgendaMode {
		t.Errorf("report mode = %q, want agenda (the report is recomputed, not stored)", served.Report.Mode)
	}
	assertNoArtifacts(t, h.outDir)
}

func TestApproveNeedsConfirmationAndIsFinal(t *testing.T) {
	h := newHarness(t, agendaOutput(meeting.UnknownOwner, 30))

	created := decode[proposalBody](t, h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON))
	if !created.Report.RequiresConfirmation {
		t.Fatal("report does not require confirmation, want the unknown-owner warning")
	}

	unconfirmed := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/approve", `{}`)
	if unconfirmed.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %s)", unconfirmed.Code, unconfirmed.Body.String())
	}
	if code := decode[errorBody](t, unconfirmed).Code; code != "confirmation_required" {
		t.Errorf("code = %q, want confirmation_required", code)
	}
	still := decode[proposalBody](t, h.do(t, http.MethodGet, "/api/v1/proposals/"+created.Proposal.ID, ""))
	if still.Proposal.State != approval.StatePending {
		t.Errorf("state after refused confirmation = %q, want pending", still.Proposal.State)
	}

	confirmed := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/approve", `{"confirm":true}`)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", confirmed.Code, confirmed.Body.String())
	}
	if state := decode[proposalBody](t, confirmed).Proposal.State; state != approval.StateApproved {
		t.Errorf("state = %q, want approved", state)
	}

	again := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/approve", `{"confirm":true}`)
	if again.Code != http.StatusConflict {
		t.Fatalf("second decision status = %d, want 409 (body %s)", again.Code, again.Body.String())
	}
	if code := decode[errorBody](t, again).Code; code != "already_decided" {
		t.Errorf("code = %q, want already_decided", code)
	}
}

func TestArtifactsNeedAnApprovalAndFollowTheMode(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		output   meeting.AgendaOrMinutes
		wantExt  string
		approve  string
		wantFile bool
	}{
		{name: "agenda writes an ICS", input: agendaJSON, output: agendaOutput("Ana", 30), wantExt: ".ics", approve: `{}`, wantFile: true},
		{name: "minutes write Markdown", input: minutesJSON, output: minutesOutput(), wantExt: ".md", approve: `{}`, wantFile: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			h := newHarness(t, testCase.output)

			created := decode[proposalBody](t, h.do(t, http.MethodPost, "/api/v1/plans", testCase.input))
			refused := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/artifacts", "")
			if refused.Code != http.StatusConflict {
				t.Fatalf("status before approval = %d, want 409 (body %s)", refused.Code, refused.Body.String())
			}
			if code := decode[errorBody](t, refused).Code; code != "not_approved" {
				t.Errorf("code = %q, want not_approved", code)
			}
			assertNoArtifacts(t, h.outDir)

			approved := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/approve", testCase.approve)
			if approved.Code != http.StatusOK {
				t.Fatalf("approve status = %d, want 200 (body %s)", approved.Code, approved.Body.String())
			}

			written := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/artifacts", "")
			if written.Code != http.StatusOK {
				t.Fatalf("artifacts status = %d, want 200 (body %s)", written.Code, written.Body.String())
			}
			body := decode[artifactsBody](t, written)
			if !body.DryRun {
				t.Error("dryRun = false, want true")
			}
			if len(body.Files) != 1 {
				t.Fatalf("files = %v, want exactly one", body.Files)
			}
			if !strings.HasSuffix(body.Files[0], testCase.wantExt) {
				t.Errorf("file %q does not end in %s", body.Files[0], testCase.wantExt)
			}
			entries, err := os.ReadDir(h.outDir)
			if err != nil {
				t.Fatalf("read artifact directory: %v", err)
			}
			if len(entries) != 1 {
				t.Errorf("artifact directory holds %d entries, want 1", len(entries))
			}
		})
	}
}

func TestRejectKeepsTheProposalOutOfDryRun(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 30))
	created := decode[proposalBody](t, h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON))

	rejected := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/reject", "")
	if rejected.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rejected.Code, rejected.Body.String())
	}
	if state := decode[proposalBody](t, rejected).Proposal.State; state != approval.StateRejected {
		t.Errorf("state = %q, want rejected", state)
	}
	refused := h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/artifacts", "")
	if refused.Code != http.StatusConflict {
		t.Fatalf("artifacts status = %d, want 409 (body %s)", refused.Code, refused.Body.String())
	}
	assertNoArtifacts(t, h.outDir)
}

func TestFailuresAreJSONWithStableCodes(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 30))
	created := decode[proposalBody](t, h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON))
	missing := "/api/v1/proposals/nope"

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{name: "unknown proposal", method: http.MethodGet, path: missing, status: http.StatusNotFound, code: "not_found"},
		{name: "unknown approve", method: http.MethodPost, path: missing + "/approve", body: `{}`, status: http.StatusNotFound, code: "not_found"},
		{name: "unknown artifacts", method: http.MethodPost, path: missing + "/artifacts", status: http.StatusNotFound, code: "not_found"},
		{name: "unknown events", method: http.MethodGet, path: missing + "/events", status: http.StatusNotFound, code: "not_found"},
		{name: "unknown api path", method: http.MethodGet, path: "/api/v1/unknown", status: http.StatusNotFound, code: "not_found"},
		{name: "wrong method", method: http.MethodDelete, path: "/api/v1/plans", status: http.StatusMethodNotAllowed, code: "method_not_allowed"},
		{name: "malformed json", method: http.MethodPost, path: "/api/v1/plans", body: "{not json", status: http.StatusBadRequest, code: "invalid_json"},
		{name: "unknown field", method: http.MethodPost, path: "/api/v1/plans", body: `{"mode":"agenda","nope":1}`, status: http.StatusBadRequest, code: "invalid_json"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := h.do(t, testCase.method, testCase.path, testCase.body)
			if recorder.Code != testCase.status {
				t.Fatalf("status = %d, want %d (body %s)", recorder.Code, testCase.status, recorder.Body.String())
			}
			if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("content type = %q, want JSON", contentType)
			}
			body := decode[errorBody](t, recorder)
			if body.Code != testCase.code {
				t.Errorf("code = %q, want %q", body.Code, testCase.code)
			}
			if strings.TrimSpace(body.Detail) == "" {
				t.Error("detail is empty, want a human explanation")
			}
		})
	}

	t.Run("oversized body", func(t *testing.T) {
		oversized := `{"meetingId":"` + strings.Repeat("a", 1<<20) + `","mode":"agenda"}`
		recorder := h.do(t, http.MethodPost, "/api/v1/plans", oversized)
		if recorder.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413 (body %s)", recorder.Code, recorder.Body.String())
		}
		if code := decode[errorBody](t, recorder).Code; code != "request_too_large" {
			t.Errorf("code = %q, want request_too_large", code)
		}
	})

	if h.generator.callCount() != 1 {
		t.Errorf("generator calls = %d, want 1: no failure path may reach the model twice", h.generator.callCount())
	}
	if created.Proposal.ID == "" {
		t.Error("the successful proposal lost its id")
	}
}

func TestEventsReplayTheRecordedTrailAndTerminate(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 30))
	created := decode[proposalBody](t, h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON))

	recorder := h.do(t, http.MethodGet, "/api/v1/proposals/"+created.Proposal.ID+"/events", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("content type = %q, want text/event-stream", contentType)
	}
	stream := recorder.Body.String()
	if !strings.Contains(stream, "data: ") {
		t.Fatalf("stream %q carries no data line", stream)
	}
	if !strings.Contains(stream, `"stage"`) {
		t.Errorf("stream %q carries no stage", stream)
	}
	if !strings.Contains(stream, created.Proposal.ID) {
		t.Errorf("stream %q never mentions the proposal id", stream)
	}
}

func TestRootServesTheEmbeddedUI(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 30))

	page := h.do(t, http.MethodGet, "/", "")
	if page.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", page.Code)
	}
	if contentType := page.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("content type = %q, want HTML", contentType)
	}
	if !strings.Contains(page.Body.String(), "Meeting agenda and minutes") {
		t.Errorf("page does not carry the product title")
	}

	script := h.do(t, http.MethodGet, "/app.js", "")
	if script.Code != http.StatusOK {
		t.Fatalf("app.js status = %d, want 200", script.Code)
	}
}

func TestOnlyArtifactsTouchTheOutputDirectory(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 30))
	first := decode[proposalBody](t, h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON))
	second := decode[proposalBody](t, h.do(t, http.MethodPost, "/api/v1/plans", agendaJSON))

	h.do(t, http.MethodGet, "/api/v1/proposals/"+first.Proposal.ID, "")
	h.do(t, http.MethodGet, "/api/v1/proposals/"+first.Proposal.ID+"/events", "")
	h.do(t, http.MethodPost, "/api/v1/proposals/"+second.Proposal.ID+"/reject", "")
	h.do(t, http.MethodPost, "/api/v1/proposals/"+second.Proposal.ID+"/artifacts", "")
	assertNoArtifacts(t, h.outDir)

	h.do(t, http.MethodPost, "/api/v1/proposals/"+first.Proposal.ID+"/approve", `{}`)
	h.do(t, http.MethodPost, "/api/v1/proposals/"+first.Proposal.ID+"/artifacts", "")

	entries, err := os.ReadDir(h.outDir)
	if err != nil {
		t.Fatalf("read artifact directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "weekly-sync.ics" {
		t.Fatalf("artifact directory = %v, want exactly weekly-sync.ics", entries)
	}
}

func TestServerServesConcurrentRequests(t *testing.T) {
	h := newHarness(t, agendaOutput("Ana", 30))
	const workers = 8

	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			input := fmt.Sprintf(`{"meetingId":"sync-%d","mode":"agenda","title":"Weekly sync","durationMinutes":30,"goals":["Ship the release"],"participants":["Ana"]}`, index)

			recorder := h.do(t, http.MethodPost, "/api/v1/plans", input)
			if recorder.Code != http.StatusOK {
				t.Errorf("plans status = %d (body %s)", recorder.Code, recorder.Body.String())
				return
			}
			var created proposalBody
			if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
				t.Errorf("decode proposal: %v", err)
				return
			}
			if _, err := h.gate.Approve(created.Proposal.ID); err != nil {
				t.Errorf("approve %s: %v", created.Proposal.ID, err)
				return
			}
			recorder = h.do(t, http.MethodPost, "/api/v1/proposals/"+created.Proposal.ID+"/artifacts", "")
			if recorder.Code != http.StatusOK {
				t.Errorf("artifacts status = %d (body %s)", recorder.Code, recorder.Body.String())
			}
		}(i)
	}
	wait.Wait()

	entries, err := os.ReadDir(h.outDir)
	if err != nil {
		t.Fatalf("read artifact directory: %v", err)
	}
	if len(entries) != workers {
		t.Fatalf("artifact count = %d, want %d", len(entries), workers)
	}
	if h.generator.callCount() != workers {
		t.Errorf("generator calls = %d, want %d", h.generator.callCount(), workers)
	}
}
