package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/agent"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

const validAgendaInput = `{"meetingId":"weekly-sync","mode":"agenda","title":"Weekly sync","durationMinutes":30,"goals":["Ship the release"],"participants":["Ana"]}`

type stubGenerator struct {
	output meeting.AgendaOrMinutes
	err    error
}

func (s stubGenerator) Generate(context.Context, meeting.MeetingPlanInput) (meeting.AgendaOrMinutes, error) {
	if s.err != nil {
		return meeting.AgendaOrMinutes{}, s.err
	}
	return s.output, nil
}

func stubNewGenerator(t *testing.T, generator stubGenerator) {
	t.Helper()
	previous := newGenerator
	newGenerator = func(context.Context, string) (agent.Generator, error) {
		if generator.err != nil {
			return nil, generator.err
		}
		return generator, nil
	}
	t.Cleanup(func() { newGenerator = previous })
}

func agendaOutput(minutes int) meeting.AgendaOrMinutes {
	return meeting.AgendaOrMinutes{
		Mode:   meeting.AgendaMode,
		Title:  "Weekly sync",
		Topics: []meeting.Topic{{Title: "Ship the release", Owner: "Ana", Minutes: minutes, GoalIndex: 0}},
	}
}

func writeInputFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write input file: %v", err)
	}
	return path
}

func execute(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestHelpDocumentsBothSubcommandsAndFlags(t *testing.T) {
	code, stdout, _ := execute(t, "-h")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	for _, want := range []string{"serve", "run", "-addr", "-out", "-input", "-mode", "-duration", "-dry-run", "-allow-remote", "GOOGLE_API_KEY", "Exit codes"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help does not mention %q", want)
		}
	}
	if code, _, _ := execute(t, "run", "-h"); code != exitOK {
		t.Errorf("run -h exit = %d, want %d", code, exitOK)
	}
}

func TestBadInvocationsAreUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "no subcommand", args: nil, want: "Usage"},
		{name: "unknown subcommand", args: []string{"deploy"}, want: "unknown subcommand"},
		{name: "unknown flag", args: []string{"run", "-nope"}, want: "flag provided but not defined"},
		{name: "run without input", args: []string{"run"}, want: "-input"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			code, _, stderr := execute(t, testCase.args...)
			if code != exitUsage {
				t.Fatalf("exit = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr, testCase.want) {
				t.Errorf("stderr %q does not mention %q", stderr, testCase.want)
			}
		})
	}
}

func TestRunRefusesToLeaveDryRunWithoutWritingAnything(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	code, _, stderr := execute(t, "run", "-input", writeInputFile(t, validAgendaInput), "-out", outDir, "-dry-run=false")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "dry run") {
		t.Errorf("stderr %q does not explain the dry-run refusal", stderr)
	}
	if _, err := os.Stat(outDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("output directory stat error = %v, want os.ErrNotExist", err)
	}
}

func TestRunWithoutAPIKeyNamesTheVariable(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	code, _, stderr := execute(t, "run", "-input", writeInputFile(t, validAgendaInput))
	if code != exitOperational {
		t.Fatalf("exit = %d, want %d (stderr %s)", code, exitOperational, stderr)
	}
	if !strings.Contains(stderr, "GOOGLE_API_KEY") {
		t.Errorf("stderr %q does not name the missing variable", stderr)
	}
}

func TestRunRejectsBadInputFiles(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
	}{
		{name: "missing file", path: filepath.Join(t.TempDir(), "absent.json")},
		{name: "malformed json", body: "{not json"},
		{name: "unknown field", body: `{"meetingId":"m","mode":"agenda","nope":1}`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			path := testCase.path
			if path == "" {
				path = writeInputFile(t, testCase.body)
			}
			code, _, stderr := execute(t, "run", "-input", path)
			if code != exitUsage {
				t.Fatalf("exit = %d, want %d (stderr %s)", code, exitUsage, stderr)
			}
			if !strings.Contains(stderr, "input") {
				t.Errorf("stderr %q does not mention the input file", stderr)
			}
		})
	}
}

func TestRunPrintsReportAndPendingProposalWithoutArtifacts(t *testing.T) {
	stubNewGenerator(t, stubGenerator{output: agendaOutput(30)})
	outDir := filepath.Join(t.TempDir(), "out")

	code, stdout, stderr := execute(t,
		"run",
		"-input", writeInputFile(t, validAgendaInput),
		"-out", outDir,
		"-mode", "agenda",
		"-duration", "30",
	)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr %s)", code, exitOK, stderr)
	}
	if !strings.Contains(stdout, `"topicMinutes": 30`) {
		t.Errorf("stdout %q does not carry the report totals", stdout)
	}
	if !strings.Contains(stdout, `"state": "pending"`) {
		t.Errorf("stdout %q does not carry the pending proposal", stdout)
	}
	if !strings.Contains(stdout, "requires a human approval") {
		t.Errorf("stdout %q does not state that the dry-run artifacts need an approval", stdout)
	}
	if _, err := os.Stat(outDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("output directory stat error = %v, want os.ErrNotExist: the headless path never approves anything", err)
	}
}

func TestRunExitsThreeWhenTheProposalFailsValidation(t *testing.T) {
	stubNewGenerator(t, stubGenerator{output: agendaOutput(120)})

	code, stdout, _ := execute(t, "run", "-input", writeInputFile(t, validAgendaInput))
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d", code, exitValidation)
	}
	if !strings.Contains(stdout, meeting.CodeDurationOverflow) {
		t.Errorf("stdout %q does not carry the blocking problem code", stdout)
	}
	if strings.Contains(stdout, `"state": "pending"`) {
		t.Errorf("stdout %q recorded a proposal that must never reach a human approval", stdout)
	}
}

func TestServeRefusesRemoteBindAndMissingKey(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
		code int
	}{
		{name: "remote bind", args: []string{"serve", "-addr", "0.0.0.0:8080"}, want: "-allow-remote", code: exitUsage},
		{name: "every interface", args: []string{"serve", "-addr", ":8080"}, want: "-allow-remote", code: exitUsage},
		{name: "invalid address", args: []string{"serve", "-addr", "nonsense"}, want: "invalid -addr", code: exitUsage},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			code, _, stderr := execute(t, testCase.args...)
			if code != testCase.code {
				t.Fatalf("exit = %d, want %d (stderr %s)", code, testCase.code, stderr)
			}
			if !strings.Contains(stderr, testCase.want) {
				t.Errorf("stderr %q does not mention %q", stderr, testCase.want)
			}
		})
	}

	t.Run("missing key", func(t *testing.T) {
		t.Setenv("GOOGLE_API_KEY", "")
		code, _, stderr := execute(t, "serve", "-addr", "127.0.0.1:0")
		if code != exitOperational {
			t.Fatalf("exit = %d, want %d (stderr %s)", code, exitOperational, stderr)
		}
		if !strings.Contains(stderr, "GOOGLE_API_KEY") {
			t.Errorf("stderr %q does not name the missing variable", stderr)
		}
	})
}

func TestServeAnswersTheUIAndShutsDownOnCancel(t *testing.T) {
	stubNewGenerator(t, stubGenerator{output: agendaOutput(30)})

	reader, writer := io.Pipe()
	defer writer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"serve", "-addr", "127.0.0.1:0", "-out", filepath.Join(t.TempDir(), "out")}, writer, writer)
	}()

	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		t.Fatal("serve printed no listening line")
	}
	line := scanner.Text()
	// An io.Pipe hands over one write at a time, so keep reading in the
	// background: the server must never block on its own progress output.
	go func() {
		for scanner.Scan() {
		}
	}()
	address := strings.TrimPrefix(line, "listening on http://")
	if address == line {
		t.Fatalf("first line %q does not announce the address", line)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + address + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if !strings.Contains(string(body), "Meeting agenda and minutes") {
		t.Errorf("served page does not carry the product title")
	}

	cancel()
	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("exit = %d, want %d", code, exitOK)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not shut down after the context was cancelled")
	}
}

func TestUsageMentionsEverySubcommandFlag(t *testing.T) {
	var stdout bytes.Buffer
	usage(&stdout)
	for _, want := range []string{"serve", "run", "-input", "-out", "GOOGLE_API_KEY"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("usage does not mention %q", want)
		}
	}
	if !strings.Contains(stdout.String(), fmt.Sprintf("%d", exitValidation)) {
		t.Error("usage does not document the validation exit code")
	}
}

// The example inputs the README documents have to keep loading into the
// contract, or the quickstart would rot silently.
func TestExampleInputsLoadIntoTheContract(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("examples", "*.json"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("examples/ holds no input file, but the README documents them")
	}

	for _, path := range paths {
		input, err := loadInput(path)
		if err != nil {
			t.Errorf("loadInput(%s) error = %v", path, err)
			continue
		}
		if strings.TrimSpace(input.MeetingID) == "" {
			t.Errorf("%s carries no meeting id", path)
		}
		if input.Mode != meeting.AgendaMode && input.Mode != meeting.MinutesMode {
			t.Errorf("%s carries mode %q, want agenda or minutes", path, input.Mode)
		}
	}
}
