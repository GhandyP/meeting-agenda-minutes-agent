package approval

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

// State is the human decision recorded for a proposal.
type State string

const (
	StatePending  State = "pending"
	StateApproved State = "approved"
	StateRejected State = "rejected"
)

// Proposal binds a request to the generated response that a human reviews.
type Proposal struct {
	ID          string                   `json:"id"`
	MeetingID   string                   `json:"meetingId"`
	Mode        meeting.Mode             `json:"mode"`
	Input       meeting.MeetingPlanInput `json:"input"`
	Output      meeting.AgendaOrMinutes  `json:"output"`
	PayloadHash string                   `json:"payloadHash"`
	State       State                    `json:"state"`
	CreatedAt   time.Time                `json:"createdAt"`
}

var (
	ErrNotFound       = errors.New("proposal not found")
	ErrNotApproved    = errors.New("proposal is not approved")
	ErrHashMismatch   = errors.New("proposal hash mismatch")
	ErrAlreadyDecided = errors.New("proposal already decided")
	ErrModeMismatch   = errors.New("input and output modes do not match")
)

// Hash returns the lowercase SHA-256 digest of the canonical JSON encoding of
// the request-response pair.
func Hash(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (string, error) {
	type canonicalInput struct {
		MeetingID       string            `json:"meetingId"`
		Mode            meeting.Mode      `json:"mode"`
		Title           string            `json:"title"`
		DurationMinutes int               `json:"durationMinutes"`
		Participants    []string          `json:"participants"`
		Goals           []string          `json:"goals"`
		Transcript      []meeting.Segment `json:"transcript"`
	}
	type canonicalOutput struct {
		Mode          meeting.Mode           `json:"mode"`
		Title         string                 `json:"title"`
		Topics        []meeting.Topic        `json:"topics"`
		Decisions     []meeting.Decision     `json:"decisions"`
		Actions       []meeting.Action       `json:"actions"`
		OpenQuestions []meeting.OpenQuestion `json:"openQuestions"`
	}
	canonical, err := json.Marshal(struct {
		Input  canonicalInput  `json:"input"`
		Output canonicalOutput `json:"output"`
	}{
		Input: canonicalInput{
			MeetingID: in.MeetingID, Mode: in.Mode, Title: in.Title,
			DurationMinutes: in.DurationMinutes, Participants: in.Participants,
			Goals: in.Goals, Transcript: in.Transcript,
		},
		Output: canonicalOutput{
			Mode: out.Mode, Title: out.Title, Topics: out.Topics,
			Decisions: out.Decisions, Actions: out.Actions, OpenQuestions: out.OpenQuestions,
		},
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// Gate stores proposals and enforces a final human decision bound to the
// exact request-response pair.
type Gate interface {
	Propose(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (Proposal, error)
	Approve(id string) (Proposal, error)
	Reject(id string) (Proposal, error)
	Get(id string) (Proposal, error)
	Authorize(id string, in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) error
}

type memoryGate struct {
	mu        sync.RWMutex
	proposals map[string]Proposal
}

// NewGate returns an in-memory gate safe for concurrent use.
func NewGate() Gate {
	return &memoryGate{proposals: make(map[string]Proposal)}
}

func (g *memoryGate) Propose(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (Proposal, error) {
	if (in.Mode != meeting.AgendaMode && in.Mode != meeting.MinutesMode) || out.Mode != in.Mode {
		return Proposal{}, ErrModeMismatch
	}
	payloadHash, err := Hash(in, out)
	if err != nil {
		return Proposal{}, err
	}

	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return Proposal{}, fmt.Errorf("generate proposal id: %w", err)
	}
	id := hex.EncodeToString(idBytes[:])
	proposal := Proposal{
		ID:          id,
		MeetingID:   in.MeetingID,
		Mode:        in.Mode,
		Input:       cloneInput(in),
		Output:      cloneOutput(out),
		PayloadHash: payloadHash,
		State:       StatePending,
		CreatedAt:   time.Now(),
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	for {
		if _, exists := g.proposals[id]; !exists {
			break
		}
		if _, err := rand.Read(idBytes[:]); err != nil {
			return Proposal{}, fmt.Errorf("generate proposal id: %w", err)
		}
		id = hex.EncodeToString(idBytes[:])
	}
	proposal.ID = id
	g.proposals[id] = cloneProposal(proposal)
	return cloneProposal(proposal), nil
}

func (g *memoryGate) Approve(id string) (Proposal, error) {
	return g.decide(id, StateApproved)
}

func (g *memoryGate) Reject(id string) (Proposal, error) {
	return g.decide(id, StateRejected)
}

func (g *memoryGate) decide(id string, decision State) (Proposal, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	proposal, exists := g.proposals[id]
	if !exists {
		return Proposal{}, ErrNotFound
	}
	if proposal.State != StatePending {
		return Proposal{}, ErrAlreadyDecided
	}
	proposal.State = decision
	g.proposals[id] = proposal
	return cloneProposal(proposal), nil
}

func (g *memoryGate) Get(id string) (Proposal, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	proposal, exists := g.proposals[id]
	if !exists {
		return Proposal{}, ErrNotFound
	}
	return cloneProposal(proposal), nil
}

func (g *memoryGate) Authorize(id string, in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) error {
	proposal, err := g.Get(id)
	if err != nil {
		return err
	}
	if proposal.State != StateApproved {
		return ErrNotApproved
	}
	payloadHash, err := Hash(in, out)
	if err != nil {
		return err
	}
	if payloadHash != proposal.PayloadHash {
		return ErrHashMismatch
	}
	return nil
}

// ArtifactFileName returns a filename using only a sanitized meeting id and an
// optional extension. It never returns a path containing a directory segment.
func ArtifactFileName(meetingID, ext string) (string, error) {
	var sanitized strings.Builder
	for _, r := range meetingID {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			sanitized.WriteRune(r)
		} else {
			sanitized.WriteByte('-')
		}
	}
	name := strings.Trim(sanitized.String(), ".-")
	if name == "" {
		return "", errors.New("meeting id is empty after sanitization")
	}
	if ext != "" {
		ext = strings.TrimPrefix(ext, ".")
		if ext == "" {
			return "", errors.New("artifact extension is empty")
		}
		for _, r := range ext {
			if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
				return "", fmt.Errorf("invalid artifact extension %q", ext)
			}
		}
		name += "." + ext
	}
	return name, nil
}

func cloneInput(in meeting.MeetingPlanInput) meeting.MeetingPlanInput {
	in.Participants = cloneSlice(in.Participants)
	in.Goals = cloneSlice(in.Goals)
	in.Transcript = cloneSlice(in.Transcript)
	return in
}

func cloneOutput(out meeting.AgendaOrMinutes) meeting.AgendaOrMinutes {
	out.Topics = cloneSlice(out.Topics)
	out.Decisions = cloneSlice(out.Decisions)
	for i := range out.Decisions {
		out.Decisions[i].Evidence = cloneSlice(out.Decisions[i].Evidence)
	}
	out.Actions = cloneSlice(out.Actions)
	for i := range out.Actions {
		out.Actions[i].Evidence = cloneSlice(out.Actions[i].Evidence)
	}
	out.OpenQuestions = cloneSlice(out.OpenQuestions)
	for i := range out.OpenQuestions {
		out.OpenQuestions[i].Evidence = cloneSlice(out.OpenQuestions[i].Evidence)
	}
	return out
}

func cloneProposal(proposal Proposal) Proposal {
	proposal.Input = cloneInput(proposal.Input)
	proposal.Output = cloneOutput(proposal.Output)
	return proposal
}

func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append(make([]T, 0, len(values)), values...)
}
