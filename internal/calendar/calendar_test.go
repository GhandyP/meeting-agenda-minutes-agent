package calendar

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/approval"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

func TestICSIsDeterministicDryRunAndUsesRequestedDuration(t *testing.T) {
	in := meeting.MeetingPlanInput{MeetingID: "team-sync", Mode: meeting.AgendaMode, Title: "Request title", DurationMinutes: 60}
	out := meeting.AgendaOrMinutes{
		Mode:  meeting.AgendaMode,
		Title: "Agenda title",
		Topics: []meeting.Topic{
			{Title: "Updates", Minutes: 25},
			{Title: "Risks", Minutes: 30},
		},
	}
	first, err := ICS(in, out)
	if err != nil {
		t.Fatalf("ICS() error = %v", err)
	}
	second, err := ICS(in, out)
	if err != nil {
		t.Fatalf("second ICS() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("ICS() is not deterministic:\n%s\n---\n%s", first, second)
	}
	text := string(first)
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"VERSION:2.0\r\n",
		"PRODID:",
		"X-DRY-RUN:TRUE\r\n",
		"SUMMARY:Agenda title\r\n",
		"DURATION:PT60M\r\n",
		"X-TOPIC:Updates (25 minutes)\r\n",
		"X-TOPIC:Risks (30 minutes)\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("ICS text does not contain %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "DTSTAMP") || strings.Contains(text, "DTSTART") || strings.Contains(text, "DTEND") {
		t.Errorf("ICS contains a clock-derived date/time property:\n%s", text)
	}
	if strings.Contains(text, "PT55M") {
		t.Errorf("ICS duration was derived from topic sum instead of request:\n%s", text)
	}
}

func TestICSSummaryFallbackEscapingAndLineFolding(t *testing.T) {
	longTitle := strings.Repeat("é", 48) + ",;\\\ncontinued"
	in := meeting.MeetingPlanInput{MeetingID: "m1", Mode: meeting.AgendaMode, Title: longTitle, DurationMinutes: 10}
	out := meeting.AgendaOrMinutes{Mode: meeting.AgendaMode, Topics: []meeting.Topic{{Title: "A,B;C\\D\nnext", Minutes: 10}}}
	data, err := ICS(in, out)
	if err != nil {
		t.Fatalf("ICS() error = %v", err)
	}
	physical := strings.Split(strings.TrimSuffix(string(data), "\r\n"), "\r\n")
	for i, line := range physical {
		if len([]byte(line)) > 75 {
			t.Errorf("physical line %d has %d octets (>75): %q", i+1, len([]byte(line)), line)
		}
		if i > 0 && strings.HasPrefix(line, " ") && len([]byte(line)) == 1 {
			t.Errorf("empty continuation line at %d", i+1)
		}
	}
	unfolded := strings.ReplaceAll(string(data), "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(longTitle, "\\", "\\\\"), ",", "\\,"), ";", "\\;"), "\n", "\\n")) {
		t.Errorf("unfolding did not restore escaped logical summary:\n%s", unfolded)
	}
	for _, escaped := range []string{`X-TOPIC:A\,B\;C\\D\nnext (10 minutes)`} {
		if !strings.Contains(unfolded, escaped) {
			t.Errorf("escaped topic missing %q:\n%s", escaped, unfolded)
		}
	}
	if !strings.Contains(string(data), "SUMMARY:") {
		t.Fatal("ICS has no SUMMARY")
	}
	if strings.Contains(string(data), "SUMMARY:\r\n") {
		t.Fatal("summary fallback did not use the request title")
	}
}

func TestICSRejectsInvalidModesDurationAndEmptyTopics(t *testing.T) {
	validIn := meeting.MeetingPlanInput{MeetingID: "m1", Mode: meeting.AgendaMode, DurationMinutes: 1}
	validOut := meeting.AgendaOrMinutes{Mode: meeting.AgendaMode, Topics: []meeting.Topic{{Title: "Topic", Minutes: 1}}}
	cases := []struct {
		name string
		in   meeting.MeetingPlanInput
		out  meeting.AgendaOrMinutes
	}{
		{name: "input mode", in: meeting.MeetingPlanInput{Mode: meeting.MinutesMode, DurationMinutes: 1}, out: meeting.AgendaOrMinutes{Mode: meeting.MinutesMode, Topics: validOut.Topics}},
		{name: "output mode", in: validIn, out: meeting.AgendaOrMinutes{Mode: meeting.MinutesMode, Topics: validOut.Topics}},
		{name: "zero duration", in: meeting.MeetingPlanInput{Mode: meeting.AgendaMode}, out: validOut},
		{name: "negative duration", in: meeting.MeetingPlanInput{Mode: meeting.AgendaMode, DurationMinutes: -1}, out: validOut},
		{name: "empty topics", in: validIn, out: meeting.AgendaOrMinutes{Mode: meeting.AgendaMode}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if data, err := ICS(tc.in, tc.out); err == nil || data != nil {
				t.Fatalf("ICS() = (%q, %v), want error and no data", data, err)
			}
		})
	}
}

func TestCalendarWriteDryRunAuthorizesBeforeFilesystemChanges(t *testing.T) {
	in := meeting.MeetingPlanInput{MeetingID: "m1", Mode: meeting.AgendaMode, DurationMinutes: 10}
	out := meeting.AgendaOrMinutes{Mode: meeting.AgendaMode, Topics: []meeting.Topic{{Title: "Topic", Minutes: 10}}}
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

func TestCalendarWriteDryRunWritesSanitizedArtifactAfterApproval(t *testing.T) {
	in := meeting.MeetingPlanInput{MeetingID: "../../etc/passwd", Mode: meeting.AgendaMode, DurationMinutes: 10}
	out := meeting.AgendaOrMinutes{Mode: meeting.AgendaMode, Topics: []meeting.Topic{{Title: "Topic", Minutes: 10}}}
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
	wantPath := filepath.Join(dir, "etc-passwd.ics")
	if path != wantPath {
		t.Fatalf("WriteDryRun() path = %q, want %q", path, wantPath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "etc-passwd.ics" {
		t.Fatalf("artifact directory entries = %v, error %v", entries, err)
	}
	if data, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(data), "BEGIN:VCALENDAR") {
		t.Fatalf("artifact content = %q, error %v", data, err)
	}
}

func TestCalendarWriteDryRunRejectsEmptyMeetingIDs(t *testing.T) {
	out := meeting.AgendaOrMinutes{Mode: meeting.AgendaMode, Topics: []meeting.Topic{{Title: "Topic", Minutes: 1}}}
	for _, tc := range []struct {
		name      string
		meetingID string
	}{
		{name: "empty", meetingID: ""},
		{name: "sanitizes empty", meetingID: "..--"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := meeting.MeetingPlanInput{MeetingID: tc.meetingID, Mode: meeting.AgendaMode, DurationMinutes: 1}
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
		return fmt.Errorf("proposal has empty id")
	}
	return nil
}
