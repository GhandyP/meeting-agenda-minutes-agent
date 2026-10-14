package agent

import (
	"context"
	"encoding/json"
	"iter"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"
)

const (
	agendaResponse  = `{"mode":"agenda","title":"Planning","topics":[{"title":"Review goals","owner":"Alex","minutes":30,"goalIndex":0}]}`
	minutesResponse = `{"mode":"minutes","title":"Review","decisions":[{"statement":"Ship the update","evidence":["seg-1"]}],"actions":[{"task":"Draft notes","owner":"Alex","dueDate":"2026-05-01","evidence":["seg-2"]}],"openQuestions":[{"question":"When is launch?","evidence":["seg-3"]}]}`
)

type fakeLLM struct {
	mu        sync.Mutex
	responses []string
	requests  []*model.LLMRequest
}

func (m *fakeLLM) Name() string { return "offline-fake" }

func (m *fakeLLM) GenerateContent(_ context.Context, req *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	m.mu.Lock()
	encoded, err := json.Marshal(req)
	if err != nil {
		m.mu.Unlock()
		panic(err)
	}
	var snapshot model.LLMRequest
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		m.mu.Unlock()
		panic(err)
	}
	m.requests = append(m.requests, &snapshot)
	response := ""
	if len(m.responses) > 0 {
		response = m.responses[0]
		m.responses = m.responses[1:]
	}
	m.mu.Unlock()

	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content: genai.NewContentFromText(response, genai.RoleModel),
		}, nil)
	}
}

func (m *fakeLLM) recordedRequests() []*model.LLMRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*model.LLMRequest(nil), m.requests...)
}

func TestNewCreatesStructuredAgendaAndMinutesBranches(t *testing.T) {
	llm := &fakeLLM{responses: []string{agendaResponse, minutesResponse}}
	generator, err := New(Config{Model: llm})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := generator.Generate(context.Background(), meeting.MeetingPlanInput{
		Mode: meeting.AgendaMode, Goals: []string{"Review launch readiness"},
		DurationMinutes: 30, Participants: []string{"Alex"},
	}); err != nil {
		t.Fatalf("Generate(agenda) error = %v", err)
	}
	if _, err := generator.Generate(context.Background(), meeting.MeetingPlanInput{
		Mode:       meeting.MinutesMode,
		Transcript: []meeting.Segment{{ID: "seg-1", Text: "We will ship the update."}},
	}); err != nil {
		t.Fatalf("Generate(minutes) error = %v", err)
	}

	requests := llm.recordedRequests()
	if len(requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requests))
	}
	for i, req := range requests {
		if req.Config == nil || req.Config.ResponseSchema == nil {
			t.Errorf("request %d has no OutputSchema", i)
			continue
		}
		schema := req.Config.ResponseSchema
		if schema.Type != genai.TypeObject {
			t.Errorf("request %d schema type = %q, want OBJECT", i, schema.Type)
		}
		for _, field := range []string{"mode", "title", "topics", "decisions", "actions", "openQuestions"} {
			if _, ok := schema.Properties[field]; !ok {
				t.Errorf("request %d schema is missing meeting.AgendaOrMinutes field %q", i, field)
			}
		}
	}
	agendaInstruction := instructionText(requests[0])
	if !strings.Contains(strings.ToLower(agendaInstruction), "time-boxed topics") {
		t.Errorf("agenda instruction does not ask for time-boxed topics: %q", agendaInstruction)
	}
	minutesInstruction := instructionText(requests[1])
	for _, phrase := range []string{"evidence-cited decisions", "actions", "open questions"} {
		if !strings.Contains(strings.ToLower(minutesInstruction), phrase) {
			t.Errorf("minutes instruction is missing %q: %q", phrase, minutesInstruction)
		}
	}

	if got := ToolNames(generator); len(got) != 0 {
		t.Errorf("tool names = %v, want no tools with side effects", got)
	}
}

func TestGenerateRoutesAndScopesPromptToMode(t *testing.T) {
	llm := &fakeLLM{responses: []string{agendaResponse, minutesResponse}}
	generator, err := New(Config{Model: llm})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	agendaInput := meeting.MeetingPlanInput{
		MeetingID: "agenda-private-id", Mode: meeting.AgendaMode, Title: "agenda-private-title",
		DurationMinutes: 45, Goals: []string{"Choose a launch date"}, Participants: []string{"Sam", "Lee"},
		Transcript: []meeting.Segment{{ID: "must-not-appear", Text: "agenda-only transcript sentinel"}},
	}
	if _, err := generator.Generate(context.Background(), agendaInput); err != nil {
		t.Fatalf("Generate(agenda) error = %v", err)
	}

	minutesInput := meeting.MeetingPlanInput{
		MeetingID: "minutes-private-id", Mode: meeting.MinutesMode, Title: "minutes-private-title",
		DurationMinutes: 99, Goals: []string{"must not appear"}, Participants: []string{"must not appear"},
		Transcript: []meeting.Segment{
			{ID: "seg-with-time", Speaker: "Sam", Timestamp: "00:01:23", Text: "We decided to launch."},
			{ID: "seg-without-time", Speaker: "Lee", Text: "I will draft the announcement."},
		},
	}
	if _, err := generator.Generate(context.Background(), minutesInput); err != nil {
		t.Fatalf("Generate(minutes) error = %v", err)
	}

	requests := llm.recordedRequests()
	if len(requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requests))
	}
	agendaPrompt := promptText(t, requests[0])
	var agenda map[string]json.RawMessage
	if err := json.Unmarshal([]byte(agendaPrompt), &agenda); err != nil {
		t.Fatalf("decode agenda prompt %q: %v", agendaPrompt, err)
	}
	if !reflect.DeepEqual(sortedKeys(agenda), []string{"durationMinutes", "goals", "participants"}) {
		t.Errorf("agenda prompt keys = %v, want only goals, durationMinutes, participants", sortedKeys(agenda))
	}
	for _, excluded := range []string{"agenda-private-id", "agenda-private-title", "must-not-appear", "agenda-only transcript sentinel"} {
		if strings.Contains(agendaPrompt, excluded) {
			t.Errorf("agenda prompt contains irrelevant value %q: %s", excluded, agendaPrompt)
		}
	}

	minutesPrompt := promptText(t, requests[1])
	var minutes map[string]json.RawMessage
	if err := json.Unmarshal([]byte(minutesPrompt), &minutes); err != nil {
		t.Fatalf("decode minutes prompt %q: %v", minutesPrompt, err)
	}
	if !reflect.DeepEqual(sortedKeys(minutes), []string{"transcript"}) {
		t.Errorf("minutes prompt keys = %v, want only transcript", sortedKeys(minutes))
	}
	var segments []map[string]json.RawMessage
	if err := json.Unmarshal(minutes["transcript"], &segments); err != nil {
		t.Fatalf("decode transcript prompt: %v", err)
	}
	if len(segments) != 2 {
		t.Fatalf("prompt transcript segments = %d, want 2", len(segments))
	}
	var id, timestamp string
	if err := json.Unmarshal(segments[0]["id"], &id); err != nil || id != "seg-with-time" {
		t.Errorf("first segment id = %q, err = %v", id, err)
	}
	if err := json.Unmarshal(segments[0]["timestamp"], &timestamp); err != nil || timestamp != "00:01:23" {
		t.Errorf("first segment timestamp = %q, err = %v", timestamp, err)
	}
	if _, ok := segments[1]["timestamp"]; ok {
		t.Errorf("timestamp without a source value was included: %s", minutesPrompt)
	}
	for _, excluded := range []string{"minutes-private-id", "minutes-private-title", "must not appear"} {
		if strings.Contains(minutesPrompt, excluded) {
			t.Errorf("minutes prompt contains irrelevant value %q: %s", excluded, minutesPrompt)
		}
	}
}

func TestGenerateRejectsEmptyOrUnknownModeBeforeModelCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode meeting.Mode
	}{{name: "empty"}, {name: "unknown", mode: meeting.Mode("later")}} {
		t.Run(tc.name, func(t *testing.T) {
			llm := &fakeLLM{}
			generator, err := New(Config{Model: llm})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := generator.Generate(context.Background(), meeting.MeetingPlanInput{Mode: tc.mode})
			if err == nil {
				t.Fatal("Generate() error = nil, want unsupported-mode error")
			}
			if !reflect.DeepEqual(got, meeting.AgendaOrMinutes{}) {
				t.Errorf("Generate() payload = %#v, want zero payload on error", got)
			}
			if calls := len(llm.recordedRequests()); calls != 0 {
				t.Errorf("model calls = %d, want 0 for invalid mode", calls)
			}
		})
	}
}

func TestGenerateDecodesFinalResponseAndRejectsInvalidJSON(t *testing.T) {
	t.Run("decodes final structured response", func(t *testing.T) {
		llm := &fakeLLM{responses: []string{agendaResponse}}
		generator, err := New(Config{Model: llm})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		got, err := generator.Generate(context.Background(), meeting.MeetingPlanInput{Mode: meeting.AgendaMode})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if got.Mode != meeting.AgendaMode || len(got.Topics) != 1 || got.Topics[0].Title != "Review goals" {
			t.Errorf("Generate() = %#v, want decoded agenda", got)
		}
	})

	t.Run("does not return zero payload as success for malformed answer", func(t *testing.T) {
		llm := &fakeLLM{responses: []string{"not valid JSON"}}
		generator, err := New(Config{Model: llm})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		got, err := generator.Generate(context.Background(), meeting.MeetingPlanInput{Mode: meeting.AgendaMode})
		if err == nil {
			t.Fatal("Generate() error = nil, want decode error")
		}
		if !reflect.DeepEqual(got, meeting.AgendaOrMinutes{}) {
			t.Errorf("Generate() payload = %#v, want zero payload on decode error", got)
		}
	})
}

func TestGenerateRejectsOutputFieldsFromOtherMode(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     meeting.Mode
		response string
	}{{
		name: "agenda with minutes fields", mode: meeting.AgendaMode,
		response: `{"mode":"agenda","title":"Bad agenda","topics":[],"decisions":[]}`,
	}, {
		name: "minutes with agenda fields", mode: meeting.MinutesMode,
		response: `{"mode":"minutes","title":"Bad minutes","topics":[]}`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			llm := &fakeLLM{responses: []string{tc.response}}
			generator, err := New(Config{Model: llm})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := generator.Generate(context.Background(), meeting.MeetingPlanInput{Mode: tc.mode})
			if err == nil {
				t.Fatal("Generate() error = nil, want mode-field exclusivity error")
			}
			if !reflect.DeepEqual(got, meeting.AgendaOrMinutes{}) {
				t.Errorf("Generate() payload = %#v, want zero payload on error", got)
			}
		})
	}
}

func TestNewGeminiModelDefaultsNameAndRequiresAPIKey(t *testing.T) {
	if _, err := NewGeminiModel(context.Background(), GeminiConfig{}); err == nil {
		t.Fatal("NewGeminiModel() error = nil, want missing-API-key error")
	} else if !strings.Contains(strings.ToLower(err.Error()), "api key") {
		t.Fatalf("NewGeminiModel() error = %v, want an API key error", err)
	}

	got, err := NewGeminiModel(context.Background(), GeminiConfig{APIKey: "offline-test-key"})
	if err != nil {
		t.Fatalf("NewGeminiModel() error = %v", err)
	}
	if got == nil {
		t.Fatal("NewGeminiModel() model = nil")
	}
	if got.Name() != "gemini-flash-latest" {
		t.Errorf("model name = %q, want gemini-flash-latest", got.Name())
	}
}

func TestGenerateLiveGemini(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY is not set")
	}

	llm, err := NewGeminiModel(context.Background(), GeminiConfig{APIKey: apiKey})
	if err != nil {
		t.Fatalf("NewGeminiModel() error = %v", err)
	}
	generator, err := New(Config{Model: llm})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	got, err := generator.Generate(context.Background(), meeting.MeetingPlanInput{
		Mode: meeting.AgendaMode, Goals: []string{"Choose one next step"},
		DurationMinutes: 15, Participants: []string{"Alex"},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.Mode != meeting.AgendaMode {
		t.Errorf("Generate().Mode = %q, want agenda", got.Mode)
	}
}

func instructionText(req *model.LLMRequest) string {
	if req.Config == nil || req.Config.SystemInstruction == nil {
		return ""
	}
	var parts []string
	for _, part := range req.Config.SystemInstruction.Parts {
		if part != nil {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func promptText(t *testing.T, req *model.LLMRequest) string {
	t.Helper()
	var prompts []string
	for _, content := range req.Contents {
		if content == nil {
			continue
		}
		for _, part := range content.Parts {
			if part != nil && part.Text != "" {
				prompts = append(prompts, part.Text)
			}
		}
	}
	if len(prompts) != 1 {
		t.Fatalf("user prompt parts = %d, want 1: %q", len(prompts), prompts)
	}
	return prompts[0]
}

func sortedKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	// The prompt schemas have small fixed key sets; sort with a local insertion sort
	// to keep the test independent of additional packages.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
