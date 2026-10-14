package minutes

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/approval"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

func TestDocumentRetainsEveryItemAndResolvedEvidenceDeterministically(t *testing.T) {
	in := meeting.MeetingPlanInput{
		MeetingID: "review-1",
		Mode:      meeting.MinutesMode,
		Title:     "Source meeting",
		Transcript: []meeting.Segment{
			{ID: "s1", Speaker: "Ari", Timestamp: "10:15", Text: "We will ship Friday."},
			{ID: "s2", Speaker: "Bo", Text: "Need a final check."},
		},
	}
	out := meeting.AgendaOrMinutes{
		Mode:          meeting.MinutesMode,
		Title:         "Release review",
		Decisions:     []meeting.Decision{{Statement: "Ship Friday", Evidence: []string{"s1"}}},
		Actions:       []meeting.Action{{Task: "Run final check", Owner: "Bo", DueDate: "Friday", Evidence: []string{"s2"}}},
		OpenQuestions: []meeting.OpenQuestion{{Question: "Who signs off?", Evidence: []string{"s1", "s2"}}},
	}
	first, err := Document(in, out)
	if err != nil {
		t.Fatalf("Document() error = %v", err)
	}
	second, err := Document(in, out)
	if err != nil {
		t.Fatalf("second Document() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("Document() is not deterministic:\n%s\n---\n%s", first, second)
	}
	text := string(first)
	for _, want := range []string{
		"# Release review",
		"DRY RUN",
		"not published",
		"## Decisions",
		"Ship Friday",
		"## Actions",
		"Run final check",
		"Owner: Bo",
		"Due date: Friday",
		"## Open questions",
		"Who signs off?",
		"Evidence id: s1",
		"Evidence id: s2",
		"Speaker: Ari",
		"Timestamp: 10:15",
		"Quote: We will ship Friday.",
		"Speaker: Bo",
		"Quote: Need a final check.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("document does not contain %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Speaker: Bo\n     > Timestamp:") {
		t.Errorf("document rendered a timestamp that was not supplied:\n%s", text)
	}
}

func TestDocumentErrorsWithoutInventingUnknownEvidence(t *testing.T) {
	in := meeting.MeetingPlanInput{Mode: meeting.MinutesMode, Transcript: []meeting.Segment{{ID: "known", Speaker: "Ari", Text: "quote"}}}
	out := meeting.AgendaOrMinutes{Mode: meeting.MinutesMode, Decisions: []meeting.Decision{{Statement: "Decision", Evidence: []string{"missing-segment"}}}}
	data, err := Document(in, out)
	if !errors.Is(err, ErrEvidenceNotFound) {
		t.Fatalf("Document() error = %v, want ErrEvidenceNotFound", err)
	}
	if !strings.Contains(err.Error(), "missing-segment") {
		t.Errorf("evidence error %q does not name the missing id", err)
	}
	if data != nil {
		t.Errorf("Document() returned data for invalid evidence: %q", data)
	}
}

func TestDocumentEmptyMinutesAndModeValidation(t *testing.T) {
	in := meeting.MeetingPlanInput{Mode: meeting.MinutesMode, Title: "Empty source"}
	out := meeting.AgendaOrMinutes{Mode: meeting.MinutesMode}
	first, err := Document(in, out)
	if err != nil {
		t.Fatalf("Document(empty) error = %v", err)
	}
	second, err := Document(in, out)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("empty document calls differ or failed: first=%q second=%q error=%v", first, second, err)
	}
	if !strings.Contains(string(first), "DRY RUN") || !strings.Contains(string(first), "not published") {
		t.Fatalf("empty document lacks dry-run marker:\n%s", first)
	}

	for _, tc := range []struct {
		name string
		in   meeting.MeetingPlanInput
		out  meeting.AgendaOrMinutes
	}{
		{name: "input mode", in: meeting.MeetingPlanInput{Mode: meeting.AgendaMode}, out: meeting.AgendaOrMinutes{Mode: meeting.MinutesMode}},
		{name: "output mode", in: meeting.MeetingPlanInput{Mode: meeting.MinutesMode}, out: meeting.AgendaOrMinutes{Mode: meeting.AgendaMode}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if data, err := Document(tc.in, tc.out); err == nil || data != nil {
				t.Fatalf("Document() = (%q, %v), want error and no document", data, err)
			}
		})
	}
}

func TestMinutesWriteDryRunAuthorizesBeforeFilesystemChanges(t *testing.T) {
	in := meeting.MeetingPlanInput{MeetingID: "minutes-1", Mode: meeting.MinutesMode, Transcript: []meeting.Segment{{ID: "s1", Text: "Evidence"}}}
	out := meeting.AgendaOrMinutes{Mode: meeting.MinutesMode, Decisions: []meeting.Decision{{Statement: "Keep", Evidence: []string{"s1"}}}}
	cases := []struct {
		name    string
		wantErr error
		setup   func(approval.Gate) (string, meeting.MeetingPlanInput, meeting.AgendaOrMinutes, error)
	}{
		{name: "wrong token", wantErr: approval.ErrNotFound, setup: func(g approval.Gate) (string, meeting.MeetingPlanInput, meeting.AgendaOrMinutes, error) {
			proposal, err := g.Propose(in, out)
			return "wrong-token", in, out, errOrID(proposal.ID, err)
		}},
		{name: "pending", wantErr: approval.ErrNotApproved, setup: func(g approval.Gate) (string, meeting.MeetingPlanInput, meeting.AgendaOrMinutes, error) {
			proposal, err := g.Propose(in, out)
			return proposal.ID, in, out, errOrID(proposal.ID, err)
		}},
		{name: "pair mismatch", wantErr: approval.ErrHashMismatch, setup: func(g approval.Gate) (string, meeting.MeetingPlanInput, meeting.AgendaOrMinutes, error) {
			proposal, err := g.Propose(in, out)
			if err == nil {
				_, err = g.Approve(proposal.ID)
			}
			changed := in
			changed.Title = "changed"
			return proposal.ID, changed, out, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gate := approval.NewGate()
			token, request, response, err := tc.setup(gate)
			if err != nil {
				t.Fatalf("setup error = %v", err)
			}
			dir := filepath.Join(t.TempDir(), "not-created")
			_, gotErr := WriteDryRun(dir, gate, token, request, response)
			if gotErr != tc.wantErr {
				t.Fatalf("WriteDryRun() error = %v, want unchanged authorization error %v", gotErr, tc.wantErr)
			}
			if _, statErr := os.Stat(dir); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("output directory stat error = %v, want os.ErrNotExist", statErr)
			}
		})
	}
}

func TestMinutesWriteDryRunWritesSanitizedArtifactAfterApproval(t *testing.T) {
	in := meeting.MeetingPlanInput{MeetingID: "../../etc/passwd", Mode: meeting.MinutesMode, Transcript: []meeting.Segment{{ID: "s1", Text: "Evidence"}}}
	out := meeting.AgendaOrMinutes{Mode: meeting.MinutesMode, Decisions: []meeting.Decision{{Statement: "Keep", Evidence: []string{"s1"}}}}
	gate := approval.NewGate()
	proposal, err := gate.Propose(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Approve(proposal.ID); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "artifacts")
	path, err := WriteDryRun(dir, gate, proposal.ID, in, out)
	if err != nil {
		t.Fatalf("WriteDryRun() error = %v", err)
	}
	wantPath := filepath.Join(dir, "etc-passwd.md")
	if path != wantPath {
		t.Fatalf("WriteDryRun() path = %q, want %q", path, wantPath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "etc-passwd.md" {
		t.Fatalf("artifact directory entries = %v, error %v", entries, err)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.Contains(string(data), "not published") {
		t.Fatalf("artifact content = %q, error %v", data, err)
	}
}

func TestMinutesWriteDryRunRejectsEmptyMeetingIDs(t *testing.T) {
	out := meeting.AgendaOrMinutes{Mode: meeting.MinutesMode}
	for _, tc := range []struct {
		name      string
		meetingID string
	}{
		{name: "empty", meetingID: ""},
		{name: "sanitizes empty", meetingID: "...---"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := meeting.MeetingPlanInput{MeetingID: tc.meetingID, Mode: meeting.MinutesMode}
			gate := approval.NewGate()
			proposal, err := gate.Propose(in, out)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := gate.Approve(proposal.ID); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "not-created")
			if _, err := WriteDryRun(dir, gate, proposal.ID, in, out); err == nil {
				t.Fatal("WriteDryRun() error = nil, want empty sanitized ID error")
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("output directory stat error = %v, want os.ErrNotExist", err)
			}
		})
	}
}

func errOrID(id string, err error) error {
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("proposal has empty id")
	}
	return nil
}
