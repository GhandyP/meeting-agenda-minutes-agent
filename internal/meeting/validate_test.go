package meeting

import (
	"slices"
	"sort"
	"testing"
)

func validAgendaInput() MeetingPlanInput {
	return MeetingPlanInput{
		MeetingID:       "m-2026-10-20-planning",
		Mode:            AgendaMode,
		Title:           "Sprint planning",
		DurationMinutes: 60,
		Participants:    []string{"ana", "bruno"},
		Goals: []string{
			"Close the scope of the release",
			"Assign the follow-ups",
		},
	}
}

func validAgendaOutput() AgendaOrMinutes {
	return AgendaOrMinutes{
		Mode:  AgendaMode,
		Title: "Sprint planning",
		Topics: []Topic{
			{Title: "Release scope", Owner: "ana", Minutes: 30, GoalIndex: 0},
			{Title: "Follow-up assignment", Owner: "bruno", Minutes: 25, GoalIndex: 1},
		},
	}
}

func validMinutesInput() MeetingPlanInput {
	return MeetingPlanInput{
		MeetingID: "m-2026-10-13-review",
		Mode:      MinutesMode,
		Title:     "Weekly review",
		Transcript: []Segment{
			{ID: "s1", Speaker: "ana", Timestamp: "00:00:12", Text: "We freeze the scope today."},
			{ID: "s2", Speaker: "bruno", Timestamp: "00:02:40", Text: "I will send the numbers on Friday."},
			{ID: "s3", Speaker: "ana", Timestamp: "00:05:03", Text: "Still open: do we need the extra reviewer?"},
		},
	}
}

func validMinutesOutput() AgendaOrMinutes {
	return AgendaOrMinutes{
		Mode:  MinutesMode,
		Title: "Weekly review",
		Decisions: []Decision{
			{Statement: "The release scope is frozen.", Evidence: []string{"s1"}},
		},
		Actions: []Action{
			{Task: "Send the numbers", Owner: "bruno", DueDate: "2026-10-16", Evidence: []string{"s2"}},
		},
		OpenQuestions: []OpenQuestion{
			{Question: "Do we need the extra reviewer?", Evidence: []string{"s3"}},
		},
	}
}

func sortedCodes(problems []Problem) []string {
	codes := make([]string, 0, len(problems))
	for _, problem := range problems {
		codes = append(codes, problem.Code)
	}
	sort.Strings(codes)
	return codes
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name             string
		in               MeetingPlanInput
		out              AgendaOrMinutes
		wantCodes        []string
		wantErrors       bool
		wantConfirmation bool
	}{
		{
			name: "agenda that fits the requested duration is clean",
			in:   validAgendaInput(),
			out:  validAgendaOutput(),
		},
		{
			name: "agenda whose time boxes exceed the duration is an error",
			in:   validAgendaInput(),
			out: func() AgendaOrMinutes {
				out := validAgendaOutput()
				out.Topics[0].Minutes = 40
				out.Topics[1].Minutes = 40
				return out
			}(),
			wantCodes:  []string{CodeDurationOverflow},
			wantErrors: true,
		},
		{
			name: "agenda with a repeated topic is an error",
			in:   validAgendaInput(),
			out: func() AgendaOrMinutes {
				out := validAgendaOutput()
				out.Topics[1].Title = "  release   SCOPE "
				return out
			}(),
			wantCodes:  []string{CodeDuplicateTopic},
			wantErrors: true,
		},
		{
			name: "agenda with an unnamed owner needs confirmation but is not an error",
			in:   validAgendaInput(),
			out: func() AgendaOrMinutes {
				out := validAgendaOutput()
				out.Topics[1].Owner = ""
				return out
			}(),
			wantCodes:        []string{CodeOwnerNeedsConfirmation},
			wantConfirmation: true,
		},
		{
			name: "agenda without goals is an error",
			in: func() MeetingPlanInput {
				in := validAgendaInput()
				in.Goals = nil
				return in
			}(),
			out:        validAgendaOutput(),
			wantCodes:  []string{CodeEmptyGoals},
			wantErrors: true,
		},
		{
			name: "agenda without a requested duration is an error",
			in: func() MeetingPlanInput {
				in := validAgendaInput()
				in.DurationMinutes = 0
				return in
			}(),
			out:        validAgendaOutput(),
			wantCodes:  []string{CodeMissingDuration},
			wantErrors: true,
		},
		{
			name: "agenda without topics is an error",
			in:   validAgendaInput(),
			out: AgendaOrMinutes{
				Mode:  AgendaMode,
				Title: "Sprint planning",
			},
			wantCodes:  []string{CodeNoTopics},
			wantErrors: true,
		},
		{
			name: "agenda carrying minutes fields is an error",
			in:   validAgendaInput(),
			out: func() AgendaOrMinutes {
				out := validAgendaOutput()
				out.Decisions = validMinutesOutput().Decisions
				out.Actions = validMinutesOutput().Actions
				out.OpenQuestions = validMinutesOutput().OpenQuestions
				return out
			}(),
			wantCodes:  []string{CodeModeFieldNotAllowed},
			wantErrors: true,
		},
		{
			name: "agenda with a zero-minute topic is an error",
			in:   validAgendaInput(),
			out: func() AgendaOrMinutes {
				out := validAgendaOutput()
				out.Topics[0].Minutes = 0
				return out
			}(),
			wantCodes:  []string{CodeTopicMinutesInvalid},
			wantErrors: true,
		},
		{
			name: "agenda topic pointing at a goal that was never supplied is an error",
			in:   validAgendaInput(),
			out: func() AgendaOrMinutes {
				out := validAgendaOutput()
				out.Topics[0].GoalIndex = 7
				return out
			}(),
			wantCodes:  []string{CodeGoalIndexOutOfRange},
			wantErrors: true,
		},
		{
			name: "minutes with evidence, owners and dates are clean",
			in:   validMinutesInput(),
			out:  validMinutesOutput(),
		},
		{
			name: "minutes without a transcript are an error",
			in: func() MeetingPlanInput {
				in := validMinutesInput()
				in.Transcript = nil
				return in
			}(),
			out:        validMinutesOutput(),
			wantCodes:  []string{CodeEmptyTranscript},
			wantErrors: true,
		},
		{
			name: "minutes carrying agenda topics are an error",
			in:   validMinutesInput(),
			out: func() AgendaOrMinutes {
				out := validMinutesOutput()
				out.Topics = validAgendaOutput().Topics
				return out
			}(),
			wantCodes:  []string{CodeModeFieldNotAllowed},
			wantErrors: true,
		},
		{
			name: "minutes item without evidence is an error",
			in:   validMinutesInput(),
			out: func() AgendaOrMinutes {
				out := validMinutesOutput()
				out.Decisions[0].Evidence = nil
				return out
			}(),
			wantCodes:  []string{CodeEvidenceMissing},
			wantErrors: true,
		},
		{
			name: "minutes citing a segment outside the transcript is an error",
			in:   validMinutesInput(),
			out: func() AgendaOrMinutes {
				out := validMinutesOutput()
				out.Actions[0].Evidence = []string{"s9"}
				return out
			}(),
			wantCodes:  []string{CodeEvidenceUnknown},
			wantErrors: true,
		},
		{
			name: "minutes citing a segment without a timestamp warn but stay valid",
			in: func() MeetingPlanInput {
				in := validMinutesInput()
				in.Transcript[2].Timestamp = ""
				return in
			}(),
			out:       validMinutesOutput(),
			wantCodes: []string{CodeEvidenceNoTimestamp},
		},
		{
			name: "minutes with an unnamed owner and no due date need confirmation",
			in:   validMinutesInput(),
			out: func() AgendaOrMinutes {
				out := validMinutesOutput()
				out.Actions[0].Owner = ""
				out.Actions[0].DueDate = ""
				return out
			}(),
			wantCodes:        []string{CodeDueDateNeedsConfirmation, CodeOwnerNeedsConfirmation},
			wantConfirmation: true,
		},
		{
			name: "minutes with a blank statement are an error",
			in:   validMinutesInput(),
			out: func() AgendaOrMinutes {
				out := validMinutesOutput()
				out.Decisions[0].Statement = "   "
				return out
			}(),
			wantCodes:  []string{CodeEmptyStatement},
			wantErrors: true,
		},
		{
			name: "minutes that extracted nothing warn but stay valid",
			in:   validMinutesInput(),
			out: AgendaOrMinutes{
				Mode:  MinutesMode,
				Title: "Weekly review",
			},
			wantCodes: []string{CodeMinutesNoItems},
		},
		{
			name: "an unknown mode is an error",
			in: func() MeetingPlanInput {
				in := validAgendaInput()
				in.Mode = Mode("weekly")
				return in
			}(),
			out:        validAgendaOutput(),
			wantCodes:  []string{CodeInputModeInvalid},
			wantErrors: true,
		},
		{
			name: "an output whose mode contradicts the request is an error",
			in:   validAgendaInput(),
			out: func() AgendaOrMinutes {
				out := validAgendaOutput()
				out.Mode = MinutesMode
				return out
			}(),
			wantCodes:  []string{CodeOutputModeMismatch},
			wantErrors: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := Validate(test.in, test.out)

			if got := sortedCodes(report.Problems); !slices.Equal(got, test.wantCodes) {
				t.Fatalf("problem codes = %v, want %v", got, test.wantCodes)
			}
			if got := report.HasErrors(); got != test.wantErrors {
				t.Fatalf("HasErrors() = %v, want %v (problems: %v)", got, test.wantErrors, report.Problems)
			}
			if got := report.RequiresConfirmation; got != test.wantConfirmation {
				t.Fatalf("RequiresConfirmation = %v, want %v (problems: %v)", got, test.wantConfirmation, report.Problems)
			}
		})
	}
}

func TestValidateReportsTimeBoxTotals(t *testing.T) {
	report := Validate(validAgendaInput(), validAgendaOutput())

	if report.TopicMinutes != 55 {
		t.Fatalf("TopicMinutes = %d, want 55", report.TopicMinutes)
	}
	if report.DurationMinutes != 60 {
		t.Fatalf("DurationMinutes = %d, want 60", report.DurationMinutes)
	}
}

func TestValidateIgnoresEvidenceOutsideMinutesMode(t *testing.T) {
	// An agenda never cites transcript segments, so an empty transcript must not
	// make it invalid.
	in := validAgendaInput()
	out := validAgendaOutput()

	if report := Validate(in, out); report.HasErrors() {
		t.Fatalf("unexpected errors for an agenda without a transcript: %v", report.Problems)
	}
}
