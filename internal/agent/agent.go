package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/genai"
)

const (
	agendaAgentName  = "meeting_agenda"
	minutesAgentName = "meeting_minutes"
	meetingAppName   = "meeting-agenda-minutes"

	agendaInstruction = "Create an agenda from the supplied goals, requested duration, and participants. " +
		"Use time-boxed topics and return only agenda fields: mode must be agenda, with a title and topics. " +
		"Give each topic a source goal index; make the topic minutes fit the requested duration. " +
		"Do not include decisions, actions, or open questions."

	minutesInstruction = "Create minutes from the supplied transcript segments only. " +
		"Return only minutes fields: mode must be minutes, with a title, decisions, actions, and open questions. " +
		"Provide evidence-cited decisions, actions and open questions, citing the supplied transcript segment ids for every claim. " +
		"Do not include agenda topics or invent details absent from the transcript."
)

// Config configures a meeting generator with an injected ADK model.
type Config struct {
	Model     model.LLM // required
	ModelName string    // optional label; defaults to Model.Name()
}

// Generator creates agendas or minutes for meeting inputs.
type Generator interface {
	Generate(ctx context.Context, in meeting.MeetingPlanInput) (meeting.AgendaOrMinutes, error)
}

// GeminiConfig configures a Gemini model client.
type GeminiConfig struct {
	APIKey    string
	ModelName string // default "gemini-flash-latest"
}

type generator struct {
	agenda    adkagent.Agent
	minutes   adkagent.Agent
	modelName string
	toolNames []string
}

// New creates agenda and minutes llmagent branches backed by the injected model.
func New(cfg Config) (Generator, error) {
	if cfg.Model == nil {
		return nil, fmt.Errorf("meeting agent model is required")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		modelName = cfg.Model.Name()
	}

	agenda, err := newBranch(agendaAgentName, "Creates time-boxed meeting agendas.", agendaInstruction, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("create agenda branch: %w", err)
	}
	minutes, err := newBranch(minutesAgentName, "Creates evidence-bearing meeting minutes.", minutesInstruction, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("create minutes branch: %w", err)
	}

	return &generator{
		agenda:    agenda,
		minutes:   minutes,
		modelName: modelName,
	}, nil
}

// NewGeminiModel constructs an ADK Gemini model with the requested name.
func NewGeminiModel(ctx context.Context, cfg GeminiConfig) (model.LLM, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("Gemini API key is required")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		modelName = "gemini-flash-latest"
	}
	llm, err := gemini.NewModel(ctx, modelName, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("create Gemini model %q: %w", modelName, err)
	}
	return llm, nil
}

// ToolNames reports the tools configured on this generator's mode branches.
// The branches intentionally have none; side effects remain behind the separate
// approval-gated application layer.
func ToolNames(g Generator) []string {
	impl, ok := g.(*generator)
	if !ok {
		return nil
	}
	return append([]string(nil), impl.toolNames...)
}

func newBranch(name, description, instruction string, llm model.LLM) (adkagent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:         name,
		Description:  description,
		Model:        llm,
		Instruction:  instruction,
		OutputSchema: agendaOrMinutesSchema(),
	})
}

func (g *generator) Generate(ctx context.Context, in meeting.MeetingPlanInput) (meeting.AgendaOrMinutes, error) {
	var branch adkagent.Agent
	switch in.Mode {
	case meeting.AgendaMode:
		branch = g.agenda
	case meeting.MinutesMode:
		branch = g.minutes
	default:
		return meeting.AgendaOrMinutes{}, fmt.Errorf("unsupported meeting mode %q", in.Mode)
	}

	prompt, err := modePrompt(in)
	if err != nil {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("encode %s prompt: %w", in.Mode, err)
	}

	meetingRunner, err := runner.NewInMemory(meetingAppName, branch)
	if err != nil {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("create meeting session runner: %w", err)
	}

	var answer string
	message := genai.NewContentFromText(prompt, genai.RoleUser)
	for event, runErr := range meetingRunner.Run(ctx, "meeting", "", message, adkagent.RunConfig{}) {
		if runErr != nil {
			return meeting.AgendaOrMinutes{}, fmt.Errorf("run %s branch with model %q: %w", in.Mode, g.modelName, runErr)
		}
		if event == nil || !event.IsFinalResponse() || event.Content == nil {
			continue
		}
		answer = contentText(event.Content)
	}
	if strings.TrimSpace(answer) == "" {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("%s branch with model %q returned no final response", in.Mode, g.modelName)
	}

	result, err := decodeModeOutput(in.Mode, answer)
	if err != nil {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("decode %s response from model %q: %w", in.Mode, g.modelName, err)
	}
	return result, nil
}

func modePrompt(in meeting.MeetingPlanInput) (string, error) {
	var input any
	switch in.Mode {
	case meeting.AgendaMode:
		input = struct {
			Goals           []string `json:"goals"`
			DurationMinutes int      `json:"durationMinutes"`
			Participants    []string `json:"participants"`
		}{
			Goals:           in.Goals,
			DurationMinutes: in.DurationMinutes,
			Participants:    in.Participants,
		}
	case meeting.MinutesMode:
		input = struct {
			Transcript []meeting.Segment `json:"transcript"`
		}{Transcript: in.Transcript}
	default:
		return "", fmt.Errorf("unsupported meeting mode %q", in.Mode)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func contentText(content *genai.Content) string {
	var text strings.Builder
	for _, part := range content.Parts {
		if part != nil && part.Text != "" {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

func decodeModeOutput(mode meeting.Mode, answer string) (meeting.AgendaOrMinutes, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(answer), &keys); err != nil {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if keys == nil {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("response must be a JSON object")
	}

	allowed := map[string]struct{}{"mode": {}, "title": {}}
	switch mode {
	case meeting.AgendaMode:
		allowed["topics"] = struct{}{}
	case meeting.MinutesMode:
		allowed["decisions"] = struct{}{}
		allowed["actions"] = struct{}{}
		allowed["openQuestions"] = struct{}{}
	default:
		return meeting.AgendaOrMinutes{}, fmt.Errorf("unsupported meeting mode %q", mode)
	}
	for key := range keys {
		if _, ok := allowed[key]; !ok {
			return meeting.AgendaOrMinutes{}, fmt.Errorf("field %q is not allowed for %s mode", key, mode)
		}
	}

	var result meeting.AgendaOrMinutes
	decoder := json.NewDecoder(bytes.NewBufferString(answer))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("decode meeting output: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return meeting.AgendaOrMinutes{}, fmt.Errorf("response contains more than one JSON value")
		}
		return meeting.AgendaOrMinutes{}, fmt.Errorf("decode trailing response data: %w", err)
	}
	if result.Mode != mode {
		return meeting.AgendaOrMinutes{}, fmt.Errorf("response mode %q does not match requested mode %q", result.Mode, mode)
	}
	return result, nil
}

func agendaOrMinutesSchema() *genai.Schema {
	stringSchema := func() *genai.Schema { return &genai.Schema{Type: genai.TypeString} }
	stringArraySchema := func() *genai.Schema {
		return &genai.Schema{Type: genai.TypeArray, Items: stringSchema()}
	}
	objectSchema := func(properties map[string]*genai.Schema, required ...string) *genai.Schema {
		return &genai.Schema{Type: genai.TypeObject, Properties: properties, Required: required}
	}
	arraySchema := func(item *genai.Schema) *genai.Schema {
		return &genai.Schema{Type: genai.TypeArray, Items: item}
	}

	return objectSchema(map[string]*genai.Schema{
		"mode": {
			Type:   genai.TypeString,
			Format: "enum",
			Enum:   []string{string(meeting.AgendaMode), string(meeting.MinutesMode)},
		},
		"title": stringSchema(),
		"topics": arraySchema(objectSchema(map[string]*genai.Schema{
			"title":     stringSchema(),
			"owner":     stringSchema(),
			"minutes":   {Type: genai.TypeInteger},
			"goalIndex": {Type: genai.TypeInteger},
		}, "title", "owner", "minutes", "goalIndex")),
		"decisions": arraySchema(objectSchema(map[string]*genai.Schema{
			"statement": stringSchema(),
			"evidence":  stringArraySchema(),
		}, "statement", "evidence")),
		"actions": arraySchema(objectSchema(map[string]*genai.Schema{
			"task":     stringSchema(),
			"owner":    stringSchema(),
			"dueDate":  stringSchema(),
			"evidence": stringArraySchema(),
		}, "task", "owner", "evidence")),
		"openQuestions": arraySchema(objectSchema(map[string]*genai.Schema{
			"question": stringSchema(),
			"evidence": stringArraySchema(),
		}, "question", "evidence")),
	}, "mode", "title")
}
