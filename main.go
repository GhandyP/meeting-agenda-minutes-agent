// Command meeting-agenda-minutes-agent runs the Google ADK (Go) meeting agent
// as a local, dry-run-only web app: meeting goals become a time-boxed agenda,
// a transcript becomes evidence-backed minutes, and nothing is published before
// a human approves the exact proposal.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/agent"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/approval"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/server"
)

const (
	apiKeyEnv    = "GOOGLE_API_KEY"
	defaultModel = "gemini-flash-latest"

	exitOK          = 0
	exitOperational = 1
	exitUsage       = 2
	exitValidation  = 3
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// newGenerator builds the production generator from the environment. Tests
// replace it to exercise the command without a model and without a key.
var newGenerator = func(ctx context.Context, modelName string) (agent.Generator, error) {
	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf("%s is not set: the ADK agent needs a Gemini key to draft a proposal (export %s=<key>)", apiKeyEnv, apiKeyEnv)
	}
	model, err := agent.NewGeminiModel(ctx, agent.GeminiConfig{APIKey: apiKey, ModelName: modelName})
	if err != nil {
		return nil, err
	}
	return agent.New(agent.Config{Model: model, ModelName: modelName})
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(stdout)
		return exitOK
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
	case "run":
		return runHeadless(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

// runServe starts the local web app and stays up until the context is done.
func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	addr := flags.String("addr", "127.0.0.1:8080", "address to listen on (loopback by default)")
	outDir := flags.String("out", "./out", "directory for the dry-run artifacts")
	allowRemote := flags.Bool("allow-remote", false, "allow a non-loopback -addr (this server has no authentication)")
	modelName := flags.String("model", defaultModel, "Gemini model name")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if err := checkAddress(*addr, *allowRemote); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitUsage
	}

	generator, err := newGenerator(ctx, *modelName)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitOperational
	}
	handler, err := server.New(server.Config{
		Generator: generator,
		Gate:      approval.NewGate(),
		OutDir:    *outDir,
		Logger:    slog.New(slog.NewTextHandler(stderr, nil)),
	})
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitOperational
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(stderr, "listen on %s: %v\n", *addr, err)
		return exitOperational
	}
	httpServer := &http.Server{Handler: handler.Handler(), ReadHeaderTimeout: 10 * time.Second}

	fmt.Fprintf(stdout, "listening on http://%s\n", listener.Addr().String())
	fmt.Fprintf(stdout, "dry run: %s receives an artifact only after you approve a proposal in the UI\n", *outDir)

	served := make(chan error, 1)
	go func() { served <- httpServer.Serve(listener) }()

	select {
	case err := <-served:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "serve: %v\n", err)
			return exitOperational
		}
		return exitOK
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(stderr, "shutdown: %v\n", err)
			return exitOperational
		}
		return exitOK
	}
}

// checkAddress keeps the unauthenticated server on loopback unless the operator
// explicitly accepts a wider bind.
func checkAddress(addr string, allowRemote bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid -addr %q: %v", addr, err)
	}
	if allowRemote {
		return nil
	}
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return fmt.Errorf("-addr %q binds every interface; pass -allow-remote only if you accept that this server has no authentication", addr)
	}
	if strings.EqualFold(trimmed, "localhost") {
		return nil
	}
	if ip := net.ParseIP(strings.Trim(trimmed, "[]")); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("-addr %q is not a loopback address; pass -allow-remote only if you accept that this server has no authentication", addr)
	}
	return nil
}

// runHeadless drafts one proposal, prints the deterministic report and stops
// before any approval: the command never grants the human decision itself.
func runHeadless(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("input", "", "path to a MeetingPlanInput JSON file (required)")
	mode := flags.String("mode", "", "override the input mode: agenda or minutes")
	duration := flags.Int("duration", 0, "override the requested duration in minutes")
	outDir := flags.String("out", "./out", "directory for the dry-run artifacts")
	modelName := flags.String("model", defaultModel, "Gemini model name")
	dryRun := flags.Bool("dry-run", true, "must stay true: v1 never leaves dry run")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if !*dryRun {
		fmt.Fprintln(stderr, "-dry-run=false is refused: v1 never leaves dry run, so no invitation is sent and no minutes are published")
		return exitUsage
	}
	if strings.TrimSpace(*inputPath) == "" {
		fmt.Fprintln(stderr, "run needs -input <file.json> holding a MeetingPlanInput")
		return exitUsage
	}

	input, err := loadInput(*inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "input %s: %v\n", *inputPath, err)
		return exitUsage
	}
	if trimmed := strings.TrimSpace(*mode); trimmed != "" {
		input.Mode = meeting.Mode(trimmed)
	}
	if *duration > 0 {
		input.DurationMinutes = *duration
	}

	generator, err := newGenerator(ctx, *modelName)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitOperational
	}
	output, err := generator.Generate(ctx, input)
	if err != nil {
		fmt.Fprintf(stderr, "generation failed: %v\n", err)
		return exitOperational
	}

	report := meeting.Validate(input, output)
	if err := writeIndented(stdout, report); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitOperational
	}
	if report.HasErrors() {
		fmt.Fprintln(stderr, "the proposal does not satisfy the meeting contract, so nothing was recorded for approval")
		return exitValidation
	}

	proposal, err := approval.NewGate().Propose(input, output)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitOperational
	}
	if err := writeIndented(stdout, proposal); err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return exitOperational
	}
	fmt.Fprintf(stdout, "proposal %s is pending: writing the dry-run artifacts under %s requires a human approval, which this command never grants by itself\n", proposal.ID, *outDir)
	return exitOK
}

func loadInput(path string) (meeting.MeetingPlanInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return meeting.MeetingPlanInput{}, err
	}
	var input meeting.MeetingPlanInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return meeting.MeetingPlanInput{}, err
	}
	return input, nil
}

func writeIndented(w io.Writer, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", encoded)
	return err
}

func usage(w io.Writer) {
	fmt.Fprint(w, `meeting-agenda-minutes-agent — dry-run meeting agendas and minutes from a Google ADK (Go) agent

Usage:
  meeting-agenda-minutes-agent serve [-addr 127.0.0.1:8080] [-out ./out] [-allow-remote] [-model gemini-flash-latest]
  meeting-agenda-minutes-agent run -input <meeting.json> [-mode agenda|minutes] [-duration 60] [-out ./out] [-model gemini-flash-latest] [-dry-run]
  meeting-agenda-minutes-agent -h | --help

serve
  -addr            address to listen on (loopback by default)
  -out             directory for the dry-run artifacts
  -allow-remote    allow a non-loopback -addr (the server has no authentication)
  -model           Gemini model name

run
  -input           path to a MeetingPlanInput JSON file (required)
  -mode            override the input mode: agenda or minutes
  -duration        override the requested duration in minutes
  -out             directory for the dry-run artifacts
  -model           Gemini model name
  -dry-run         must stay true: v1 never leaves dry run

Dry run:
  Nothing is published and no invitation is sent. An agenda produces an ICS
  preview, minutes produce a Markdown document, and either one is written
  locally and only after a human approves the exact proposal.

Environment:
  GOOGLE_API_KEY   required by both subcommands: the ADK agent calls Gemini to
                   draft the proposal, and the key is never written to disk

Exit codes:
  0  success
  1  operational error (missing key, model failure, unusable listener)
  2  usage error (bad flags, unreadable input file, refused remote bind)
  3  the proposal failed deterministic validation, so it was not recorded
`)
}
