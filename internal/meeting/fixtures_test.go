package meeting_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

func TestLoadCasesMatchValidation(t *testing.T) {
	cases, err := meeting.LoadCases(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatalf("LoadCases() error = %v", err)
	}

	wantNames := []string{
		"agenda_duplicate_topics",
		"agenda_duration_overflow",
		"agenda_missing_owner",
		"minutes_empty_transcript",
		"minutes_missing_owner",
		"minutes_missing_timestamp",
		"mode_selection_agenda",
		"mode_selection_minutes",
	}
	gotNames := make([]string, 0, len(cases))
	for _, testCase := range cases {
		gotNames = append(gotNames, testCase.Name)
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("case names = %v, want filename order %v", gotNames, wantNames)
	}

	for _, testCase := range cases {
		t.Run(testCase.Name, func(t *testing.T) {
			report := meeting.Validate(testCase.Input, testCase.Output)
			gotCodes := make([]string, 0, len(report.Problems))
			for _, problem := range report.Problems {
				gotCodes = append(gotCodes, problem.Code)
			}
			if !slices.Equal(gotCodes, testCase.WantProblemCodes) {
				t.Errorf("problem codes = %v, want %v", gotCodes, testCase.WantProblemCodes)
			}
			if report.TopicMinutes != testCase.WantTopicMinutes {
				t.Errorf("TopicMinutes = %d, want %d", report.TopicMinutes, testCase.WantTopicMinutes)
			}
			if got := report.HasErrors(); got != testCase.WantHasErrors {
				t.Errorf("HasErrors() = %t, want %t", got, testCase.WantHasErrors)
			}
		})
	}
}

func TestLoadCasesReadsTopLevelJSONInFilenameOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.json", "a.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"name":"`+strings.TrimSuffix(name, ".json")+`"}`), 0o600); err != nil {
			t.Fatalf("WriteFile(%q): %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o700); err != nil {
		t.Fatalf("Mkdir(nested): %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "ignored.json"), []byte(`{"name":"ignored"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(nested/ignored.json): %v", err)
	}

	cases, err := meeting.LoadCases(dir)
	if err != nil {
		t.Fatalf("LoadCases() error = %v", err)
	}
	got := make([]string, 0, len(cases))
	for _, testCase := range cases {
		got = append(got, testCase.Name)
	}
	if want := []string{"a", "z"}; !slices.Equal(got, want) {
		t.Fatalf("case names = %v, want filename order %v", got, want)
	}
}

func TestLoadCasesReportsMalformedFilename(t *testing.T) {
	dir := t.TempDir()
	const filename = "broken-fixture.json"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(`{"name":`), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", filename, err)
	}

	_, err := meeting.LoadCases(dir)
	if err == nil {
		t.Fatal("LoadCases() error = nil, want malformed fixture error")
	}
	if !strings.Contains(err.Error(), filename) {
		t.Fatalf("LoadCases() error = %q, want it to name %q", err, filename)
	}
}
