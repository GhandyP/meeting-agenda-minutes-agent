package approval

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

func TestHashBindsCanonicalInputOutputPair(t *testing.T) {
	in := meeting.MeetingPlanInput{
		MeetingID:       "planning-1",
		Mode:            meeting.AgendaMode,
		Title:           "Planning",
		DurationMinutes: 60,
		Goals:           []string{"launch"},
	}
	out := meeting.AgendaOrMinutes{
		Mode:   meeting.AgendaMode,
		Title:  "Launch planning",
		Topics: []meeting.Topic{{Title: "Risks", Owner: "Ari", Minutes: 30, GoalIndex: 0}},
	}

	first, err := Hash(in, out)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	second, err := Hash(in, out)
	if err != nil {
		t.Fatalf("Hash() second call error = %v", err)
	}
	if first != second {
		t.Fatalf("equal pairs hashed differently: %q != %q", first, second)
	}
	if len(first) != 64 || first != strings.ToLower(first) {
		t.Fatalf("Hash() = %q, want 64 lowercase hex characters", first)
	}
	for _, r := range first {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			t.Fatalf("Hash() = %q, contains non-hex character %q", first, r)
		}
	}

	changedInput := in
	changedInput.Title = "Changed request"
	inputHash, err := Hash(changedInput, out)
	if err != nil {
		t.Fatalf("Hash() with changed input error = %v", err)
	}
	if inputHash == first {
		t.Fatal("changing an input field did not change the hash")
	}

	changedOutput := out
	changedOutput.Title = "Changed proposal"
	outputHash, err := Hash(in, changedOutput)
	if err != nil {
		t.Fatalf("Hash() with changed output error = %v", err)
	}
	if outputHash == first {
		t.Fatal("changing an output field did not change the hash")
	}

	emptyParticipants := in
	emptyParticipants.Participants = []string{}
	emptyHash, err := Hash(emptyParticipants, out)
	if err != nil {
		t.Fatalf("Hash() with empty input slice error = %v", err)
	}
	if emptyHash == first {
		t.Fatal("changing nil to empty input slice did not change the hash")
	}
}

func TestProposeStoresPendingDefensiveCopyAndUniqueHexID(t *testing.T) {
	gate := NewGate()
	in := meeting.MeetingPlanInput{
		MeetingID:       "meeting-17",
		Mode:            meeting.MinutesMode,
		Title:           "Review",
		DurationMinutes: 60,
		Participants:    []string{"Ari"},
		Goals:           []string{"review"},
		Transcript:      []meeting.Segment{{ID: "s1", Speaker: "Ari", Timestamp: "10:00", Text: "ship"}},
	}
	out := meeting.AgendaOrMinutes{
		Mode:          meeting.MinutesMode,
		Title:         "Review minutes",
		Decisions:     []meeting.Decision{{Statement: "Ship", Evidence: []string{"s1"}}},
		Actions:       []meeting.Action{{Task: "Publish draft", Owner: "Ari", DueDate: "Friday", Evidence: []string{"s1"}}},
		OpenQuestions: []meeting.OpenQuestion{{Question: "When?", Evidence: []string{"s1"}}},
	}

	first, err := gate.Propose(in, out)
	if err != nil {
		t.Fatalf("Propose() error = %v", err)
	}
	second, err := gate.Propose(in, out)
	if err != nil {
		t.Fatalf("second Propose() error = %v", err)
	}
	for _, id := range []string{first.ID, second.ID} {
		decoded, err := hex.DecodeString(id)
		if err != nil || len(decoded) == 0 {
			t.Fatalf("proposal ID %q is not non-empty hex (decoded %x, error %v)", id, decoded, err)
		}
	}
	if first.ID == second.ID {
		t.Fatalf("two Propose() calls returned the same ID %q", first.ID)
	}
	if first.State != StatePending || first.MeetingID != in.MeetingID || first.Mode != in.Mode {
		t.Fatalf("proposal metadata = state %q, meeting %q, mode %q", first.State, first.MeetingID, first.Mode)
	}
	if first.CreatedAt.IsZero() {
		t.Fatal("proposal CreatedAt is zero")
	}
	wantHash, err := Hash(in, out)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if first.PayloadHash != wantHash {
		t.Fatalf("PayloadHash = %q, want %q", first.PayloadHash, wantHash)
	}

	in.Participants[0] = "Mallory"
	in.Goals[0] = "changed"
	in.Transcript[0].Text = "changed"
	out.Decisions[0].Statement = "changed"
	out.Decisions[0].Evidence[0] = "missing"
	out.Actions[0].Task = "changed"
	out.Actions[0].Evidence[0] = "missing"
	out.OpenQuestions[0].Question = "changed"
	out.OpenQuestions[0].Evidence[0] = "missing"
	stored, err := gate.Get(first.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Input.Participants[0] != "Ari" || stored.Input.Goals[0] != "review" || stored.Input.Transcript[0].Text != "ship" {
		t.Fatalf("stored input changed after caller mutation: %+v", stored.Input)
	}
	if stored.Output.Decisions[0].Statement != "Ship" || stored.Output.Decisions[0].Evidence[0] != "s1" ||
		stored.Output.Actions[0].Task != "Publish draft" || stored.Output.Actions[0].Evidence[0] != "s1" ||
		stored.Output.OpenQuestions[0].Question != "When?" || stored.Output.OpenQuestions[0].Evidence[0] != "s1" {
		t.Fatalf("stored output changed after caller mutation: %+v", stored.Output)
	}
}

func TestProposeRejectsMismatchedModes(t *testing.T) {
	gate := NewGate()
	validInput := meeting.MeetingPlanInput{MeetingID: "m1", Mode: meeting.AgendaMode}
	validOutput := meeting.AgendaOrMinutes{Mode: meeting.AgendaMode}
	cases := []struct {
		name string
		in   meeting.MeetingPlanInput
		out  meeting.AgendaOrMinutes
	}{
		{name: "unknown input mode", in: meeting.MeetingPlanInput{MeetingID: "m1", Mode: "unknown"}, out: validOutput},
		{name: "output mode differs", in: validInput, out: meeting.AgendaOrMinutes{Mode: meeting.MinutesMode}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proposal, err := gate.Propose(tc.in, tc.out)
			if err != ErrModeMismatch {
				t.Fatalf("Propose() error = %v, want ErrModeMismatch", err)
			}
			if !reflect.DeepEqual(proposal, Proposal{}) {
				t.Fatalf("Propose() proposal = %+v, want zero value", proposal)
			}
		})
	}
}

func TestDecisionsAreFinalAndUnknownIDsAreNotFound(t *testing.T) {
	for _, decision := range []struct {
		name string
		call func(Gate, string) (Proposal, error)
		want State
	}{
		{name: "approve", call: Gate.Approve, want: StateApproved},
		{name: "reject", call: Gate.Reject, want: StateRejected},
	} {
		t.Run(decision.name, func(t *testing.T) {
			gate := NewGate()
			proposal, err := gate.Propose(meeting.MeetingPlanInput{MeetingID: "m1", Mode: meeting.AgendaMode}, meeting.AgendaOrMinutes{Mode: meeting.AgendaMode})
			if err != nil {
				t.Fatalf("Propose() error = %v", err)
			}
			updated, err := decision.call(gate, proposal.ID)
			if err != nil || updated.State != decision.want {
				t.Fatalf("first decision = (%+v, %v), want state %q", updated, err, decision.want)
			}
			if _, err := gate.Approve(proposal.ID); err != ErrAlreadyDecided {
				t.Fatalf("second Approve() error = %v, want ErrAlreadyDecided", err)
			}
			if _, err := gate.Reject(proposal.ID); err != ErrAlreadyDecided {
				t.Fatalf("second Reject() error = %v, want ErrAlreadyDecided", err)
			}
			stored, err := gate.Get(proposal.ID)
			if err != nil || stored.State != decision.want {
				t.Fatalf("Get() after repeated decision = (%+v, %v), want state %q", stored, err, decision.want)
			}
		})
	}

	gate := NewGate()
	if _, err := gate.Approve("unknown"); err != ErrNotFound {
		t.Errorf("Approve(unknown) error = %v, want ErrNotFound", err)
	}
	if _, err := gate.Reject("unknown"); err != ErrNotFound {
		t.Errorf("Reject(unknown) error = %v, want ErrNotFound", err)
	}
	if _, err := gate.Get("unknown"); err != ErrNotFound {
		t.Errorf("Get(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestAuthorizeBindsApprovalToExactRequestResponsePair(t *testing.T) {
	gate := NewGate()
	in := meeting.MeetingPlanInput{MeetingID: "m1", Mode: meeting.MinutesMode, Title: "Review", Transcript: []meeting.Segment{{ID: "s1", Text: "Ship it"}}}
	out := meeting.AgendaOrMinutes{Mode: meeting.MinutesMode, Title: "Minutes", Decisions: []meeting.Decision{{Statement: "Ship", Evidence: []string{"s1"}}}}
	proposal, err := gate.Propose(in, out)
	if err != nil {
		t.Fatalf("Propose() error = %v", err)
	}
	if err := gate.Authorize(proposal.ID, in, out); err != ErrNotApproved {
		t.Fatalf("Authorize(pending) error = %v, want ErrNotApproved", err)
	}
	if _, err := gate.Reject(proposal.ID); err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if err := gate.Authorize(proposal.ID, in, out); err != ErrNotApproved {
		t.Fatalf("Authorize(rejected) error = %v, want ErrNotApproved", err)
	}
	if _, err := gate.Get(proposal.ID); err != nil {
		t.Fatalf("Get() after authorization failures: %v", err)
	}

	approved, err := gate.Propose(in, out)
	if err != nil {
		t.Fatalf("second Propose() error = %v", err)
	}
	if _, err := gate.Approve(approved.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if err := gate.Authorize(approved.ID, in, out); err != nil {
		t.Fatalf("Authorize(matching approved pair) error = %v", err)
	}

	changedInput := in
	changedInput.Title = "Different request"
	if err := gate.Authorize(approved.ID, changedInput, out); err != ErrHashMismatch {
		t.Errorf("Authorize(changed input) error = %v, want ErrHashMismatch", err)
	}
	changedOutput := out
	changedOutput.Title = "Different response"
	if err := gate.Authorize(approved.ID, in, changedOutput); err != ErrHashMismatch {
		t.Errorf("Authorize(changed output) error = %v, want ErrHashMismatch", err)
	}
	if err := gate.Authorize("unknown", in, out); err != ErrNotFound {
		t.Errorf("Authorize(unknown) error = %v, want ErrNotFound", err)
	}
	stored, err := gate.Get(approved.ID)
	if err != nil || stored.State != StateApproved {
		t.Fatalf("Get() after authorization failures = (%+v, %v), want approved", stored, err)
	}
}

func TestArtifactFileNameSanitizesMeetingIDAndRejectsEmptyNames(t *testing.T) {
	for _, tc := range []struct {
		meetingID string
		ext       string
		want      string
	}{
		{meetingID: "../../etc/passwd", ext: "ics", want: "etc-passwd.ics"},
		{meetingID: "A_b-1.2", ext: ".md", want: "A_b-1.2.md"},
		{meetingID: "a b😀c", ext: "ics", want: "a-b-c.ics"},
	} {
		got, err := ArtifactFileName(tc.meetingID, tc.ext)
		if err != nil || got != tc.want {
			t.Errorf("ArtifactFileName(%q, %q) = (%q, %v), want %q", tc.meetingID, tc.ext, got, err, tc.want)
		}
	}
	for _, meetingID := range []string{"", "...---", "/\\"} {
		if got, err := ArtifactFileName(meetingID, "ics"); err == nil || got != "" {
			t.Errorf("ArtifactFileName(%q) = (%q, %v), want an error and empty name", meetingID, got, err)
		}
	}
}

func TestGateConcurrentUseKeepsRecordsIntact(t *testing.T) {
	const workers = 32
	gate := NewGate()
	errs := make(chan error, workers)
	ids := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := meeting.MeetingPlanInput{MeetingID: fmt.Sprintf("meeting-%d", i), Mode: meeting.AgendaMode, Goals: []string{fmt.Sprintf("goal-%d", i)}}
			out := meeting.AgendaOrMinutes{Mode: meeting.AgendaMode, Topics: []meeting.Topic{{Title: fmt.Sprintf("topic-%d", i), Owner: "Ari", Minutes: i + 1}}}
			proposal, err := gate.Propose(in, out)
			if err != nil {
				errs <- fmt.Errorf("Propose(%d): %w", i, err)
				return
			}
			if _, err := gate.Approve(proposal.ID); err != nil {
				errs <- fmt.Errorf("Approve(%d): %w", i, err)
				return
			}
			stored, err := gate.Get(proposal.ID)
			if err != nil {
				errs <- fmt.Errorf("Get(%d): %w", i, err)
				return
			}
			if stored.State != StateApproved || stored.Input.MeetingID != in.MeetingID || stored.Output.Topics[0].Title != out.Topics[0].Title {
				errs <- fmt.Errorf("record %d was corrupted: %+v", i, stored)
				return
			}
			if err := gate.Authorize(proposal.ID, in, out); err != nil {
				errs <- fmt.Errorf("Authorize(%d): %w", i, err)
				return
			}
			ids <- proposal.ID
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	close(ids)
	count := 0
	for id := range ids {
		count++
		if _, err := gate.Get(id); err != nil {
			t.Errorf("Get(%q) after concurrent operations: %v", id, err)
		}
	}
	if count != workers {
		t.Fatalf("concurrent workers completed %d records, want %d", count, workers)
	}
}
