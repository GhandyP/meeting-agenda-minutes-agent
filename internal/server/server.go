// Package server exposes the meeting agent as a versioned JSON API plus the
// embedded dry-run UI.
//
// The server owns no business rule of its own: it calls the injected generator,
// the deterministic meeting validator and the human approval gate, and it
// writes to disk only through the calendar or minutes dry-run writer of a
// proposal a human approved.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/agent"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/approval"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/calendar"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/minutes"
	"github.com/GhandyP/meeting-agenda-minutes-agent/web"
)

// maxRequestBytes bounds one request body.
const maxRequestBytes = 1 << 20

// Stable machine-readable error codes. They are part of the API contract.
const (
	codeInvalidJSON          = "invalid_json"
	codeRequestTooLarge      = "request_too_large"
	codeMethodNotAllowed     = "method_not_allowed"
	codeValidationFailed     = "validation_failed"
	codeNotFound             = "not_found"
	codeConfirmationRequired = "confirmation_required"
	codeAlreadyDecided       = "already_decided"
	codeNotApproved          = "not_approved"
	codeGenerationFailed     = "generation_failed"
	codeArtifactsFailed      = "artifacts_failed"
	codeInternalError        = "internal_error"
)

// Config wires the server to its collaborators.
type Config struct {
	Generator agent.Generator
	Gate      approval.Gate
	OutDir    string       // dry-run artifact directory
	Logger    *slog.Logger // optional; defaults to a discard logger
}

// Stage is one recorded step of a proposal's progress.
type Stage struct {
	Stage      string `json:"stage"`
	ProposalID string `json:"proposalId,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// Server answers the JSON API and the embedded UI.
type Server struct {
	generator agent.Generator
	gate      approval.Gate
	outDir    string
	logger    *slog.Logger

	mu     sync.Mutex
	stages map[string][]Stage // keyed by meeting id
}

// New returns a server over the given collaborators.
func New(cfg Config) (*Server, error) {
	if cfg.Generator == nil {
		return nil, errors.New("server: a generator is required")
	}
	if cfg.Gate == nil {
		return nil, errors.New("server: an approval gate is required")
	}
	if strings.TrimSpace(cfg.OutDir) == "" {
		return nil, errors.New("server: the dry-run output directory is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Server{
		generator: cfg.Generator,
		gate:      cfg.Gate,
		outDir:    cfg.OutDir,
		logger:    logger,
		stages:    make(map[string][]Stage),
	}, nil
}

// Handler returns the whole HTTP surface: the JSON API under /api/v1 and the
// embedded UI at the root.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/plans", s.plans)
	mux.HandleFunc("/api/v1/proposals/{id}", s.proposal)
	mux.HandleFunc("/api/v1/proposals/{id}/approve", s.approve)
	mux.HandleFunc("/api/v1/proposals/{id}/reject", s.reject)
	mux.HandleFunc("/api/v1/proposals/{id}/artifacts", s.artifacts)
	mux.HandleFunc("/api/v1/proposals/{id}/events", s.events)
	mux.HandleFunc("/api/", s.unknownAPI)
	mux.Handle("/", web.Handler())
	return mux
}

// plans generates, validates and records a proposal. It never writes a file.
func (s *Server) plans(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	var input meeting.MeetingPlanInput
	if !decodeJSON(w, r, &input) {
		return
	}

	key := stageKey(input.MeetingID)
	s.record(key, Stage{Stage: "received"})

	output, err := s.generator.Generate(r.Context(), input)
	if err != nil {
		s.record(key, Stage{Stage: "generation_failed", Detail: err.Error()})
		s.logger.Error("generate proposal", "error", err)
		writeError(w, http.StatusBadGateway, codeGenerationFailed, err.Error())
		return
	}
	s.record(key, Stage{Stage: "generated"})

	report := meeting.Validate(input, output)
	if report.HasErrors() {
		s.record(key, Stage{Stage: "blocked_by_validation", Detail: firstErrorDetail(report)})
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Code:   codeValidationFailed,
			Detail: "the proposal does not satisfy the meeting contract, so it cannot reach a human approval",
			Report: &report,
		})
		return
	}

	proposal, err := s.gate.Propose(input, output)
	if err != nil {
		s.logger.Error("record proposal", "error", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, err.Error())
		return
	}
	s.record(key, Stage{Stage: "proposed", ProposalID: proposal.ID})
	writeJSON(w, http.StatusOK, proposalResponse{Proposal: proposal, Report: report})
}

// proposal serves one stored proposal with its recomputed report.
func (s *Server) proposal(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	proposal, ok := s.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, proposalResponse{
		Proposal: proposal,
		Report:   meeting.Validate(proposal.Input, proposal.Output),
	})
}

// approve records the human decision, refusing while the report still needs an
// explicit confirmation of owners and deadlines.
func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	proposal, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	report := meeting.Validate(proposal.Input, proposal.Output)
	if report.RequiresConfirmation && !body.Confirm {
		writeJSON(w, http.StatusConflict, errorResponse{
			Code:   codeConfirmationRequired,
			Detail: "a human has to confirm the owners and deadlines this proposal reports before approving it",
			Report: &report,
		})
		return
	}

	approved, err := s.gate.Approve(proposal.ID)
	if err != nil {
		s.writeDecisionError(w, err, proposal.ID)
		return
	}
	writeJSON(w, http.StatusOK, proposalResponse{Proposal: approved, Report: report})
}

// reject records the human decision and keeps the proposal out of dry run.
func (s *Server) reject(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	proposal, ok := s.lookup(w, r)
	if !ok {
		return
	}
	rejected, err := s.gate.Reject(proposal.ID)
	if err != nil {
		s.writeDecisionError(w, err, proposal.ID)
		return
	}
	writeJSON(w, http.StatusOK, proposalResponse{
		Proposal: rejected,
		Report:   meeting.Validate(rejected.Input, rejected.Output),
	})
}

// artifacts is the only route that writes to disk, and only for an approved
// proposal whose stored input and output still match the approval.
func (s *Server) artifacts(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodPost) {
		return
	}
	proposal, ok := s.lookup(w, r)
	if !ok {
		return
	}
	report := meeting.Validate(proposal.Input, proposal.Output)
	if proposal.State != approval.StateApproved {
		writeJSON(w, http.StatusConflict, errorResponse{
			Code:   codeNotApproved,
			Detail: fmt.Sprintf("proposal %s is %s: only an approved proposal may write a dry-run artifact", proposal.ID, proposal.State),
			Report: &report,
		})
		return
	}

	var (
		path string
		err  error
	)
	switch proposal.Mode {
	case meeting.AgendaMode:
		path, err = calendar.WriteDryRun(s.outDir, s.gate, proposal.ID, proposal.Input, proposal.Output)
	case meeting.MinutesMode:
		path, err = minutes.WriteDryRun(s.outDir, s.gate, proposal.ID, proposal.Input, proposal.Output)
	default:
		err = fmt.Errorf("proposal %s carries unsupported mode %q", proposal.ID, proposal.Mode)
	}
	if err != nil {
		s.logger.Error("write dry-run artifact", "proposal", proposal.ID, "error", err)
		writeError(w, http.StatusInternalServerError, codeArtifactsFailed, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, artifactsResponse{Files: []string{path}, DryRun: true})
}

// events replays the recorded progress trail of one proposal as SSE and closes.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if !allowMethod(w, r, http.MethodGet) {
		return
	}
	proposal, ok := s.lookup(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, codeInternalError, "the response writer cannot stream")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for _, stage := range s.recorded(stageKey(proposal.MeetingID)) {
		payload, err := json.Marshal(stage)
		if err != nil {
			s.logger.Error("encode progress stage", "error", err)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}
}

func (s *Server) unknownAPI(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, codeNotFound, "no API route matches this path")
}

// lookup resolves the {id} path segment to a stored proposal.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (approval.Proposal, bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusNotFound, codeNotFound, "the request carries no proposal id")
		return approval.Proposal{}, false
	}
	proposal, err := s.gate.Get(id)
	if err != nil {
		if errors.Is(err, approval.ErrNotFound) {
			writeError(w, http.StatusNotFound, codeNotFound, fmt.Sprintf("no proposal %s exists", id))
			return approval.Proposal{}, false
		}
		s.logger.Error("read proposal", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, err.Error())
		return approval.Proposal{}, false
	}
	return proposal, true
}

func (s *Server) writeDecisionError(w http.ResponseWriter, err error, id string) {
	switch {
	case errors.Is(err, approval.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, fmt.Sprintf("no proposal %s exists", id))
	case errors.Is(err, approval.ErrAlreadyDecided):
		writeError(w, http.StatusConflict, codeAlreadyDecided, fmt.Sprintf("proposal %s was already decided", id))
	default:
		s.logger.Error("decide proposal", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, codeInternalError, err.Error())
	}
}

func (s *Server) record(key string, stage Stage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stages[key] = append(s.stages[key], stage)
}

func (s *Server) recorded(key string) []Stage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Stage(nil), s.stages[key]...)
}

func stageKey(meetingID string) string {
	if key := strings.TrimSpace(meetingID); key != "" {
		return key
	}
	return "unidentified-meeting"
}

type proposalResponse struct {
	Proposal approval.Proposal `json:"proposal"`
	Report   meeting.Report    `json:"report"`
}

type artifactsResponse struct {
	Files  []string `json:"files"`
	DryRun bool     `json:"dryRun"`
}

type errorResponse struct {
	Code   string          `json:"code"`
	Detail string          `json:"detail"`
	Report *meeting.Report `json:"report,omitempty"`
}

func allowMethod(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for _, method := range allowed {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed,
		fmt.Sprintf("%s is not allowed on %s", r.Method, r.URL.Path))
	return false
}

// decodeJSON reads exactly one JSON object, refusing unknown fields, trailing
// data and anything above maxRequestBytes.
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, codeRequestTooLarge,
				fmt.Sprintf("the request body exceeds %d bytes", maxRequestBytes))
			return false
		}
		writeError(w, http.StatusBadRequest, codeInvalidJSON, err.Error())
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, codeInvalidJSON, "the request body must hold exactly one JSON object")
		return false
	}
	return true
}

func firstErrorDetail(report meeting.Report) string {
	for _, problem := range report.Problems {
		if problem.Severity == meeting.SeverityError {
			return problem.Detail
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, detail string) {
	writeJSON(w, status, errorResponse{Code: code, Detail: detail})
}
