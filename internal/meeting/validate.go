package meeting

import (
	"fmt"
	"strings"
)

// Severity separates a violation that blocks approval from an observation that
// a human still has to resolve.
type Severity string

const (
	// SeverityError marks a proposal that must not be approved.
	SeverityError Severity = "error"
	// SeverityWarning marks a proposal that is valid but needs a human decision.
	SeverityWarning Severity = "warning"
)

// Stable, machine-readable problem codes. The web app, the headless command and
// the tests all switch on these values, so they are part of the contract.
const (
	CodeInputModeInvalid         = "input_mode_invalid"
	CodeOutputModeMismatch       = "output_mode_mismatch"
	CodeModeFieldNotAllowed      = "mode_field_not_allowed"
	CodeEmptyGoals               = "empty_goals"
	CodeEmptyTranscript          = "empty_transcript"
	CodeMissingDuration          = "missing_duration"
	CodeNoTopics                 = "no_topics"
	CodeTopicMinutesInvalid      = "topic_minutes_invalid"
	CodeDurationOverflow         = "duration_overflow"
	CodeDuplicateTopic           = "duplicate_topic"
	CodeGoalIndexOutOfRange      = "goal_index_out_of_range"
	CodeOwnerNeedsConfirmation   = "owner_needs_confirmation"
	CodeDueDateNeedsConfirmation = "due_date_needs_confirmation"
	CodeEmptyStatement           = "empty_statement"
	CodeEvidenceMissing          = "evidence_missing"
	CodeEvidenceUnknown          = "evidence_unknown"
	CodeEvidenceNoTimestamp      = "evidence_timestamp_missing"
	CodeMinutesNoItems           = "minutes_no_items"
)

// Problem is one deterministic finding about a proposal.
type Problem struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Field    string   `json:"field"`
	Detail   string   `json:"detail"`
}

// Report is the outcome of validating one proposal. Problems are appended in a
// fixed evaluation order, so the same input always produces the same slice.
type Report struct {
	Mode                 Mode      `json:"mode"`
	DurationMinutes      int       `json:"durationMinutes"`
	TopicMinutes         int       `json:"topicMinutes"`
	Problems             []Problem `json:"problems"`
	RequiresConfirmation bool      `json:"requiresConfirmation"`
}

// HasErrors reports whether any problem blocks approval.
func (r Report) HasErrors() bool {
	for _, problem := range r.Problems {
		if problem.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Validate applies every deterministic rule of the contract to one proposal.
//
// Rules are grouped so that a root cause is reported once: when the input is
// missing goals or transcript segments, the checks that resolve against them
// stay quiet instead of adding consequential noise.
func Validate(in MeetingPlanInput, out AgendaOrMinutes) Report {
	report := Report{
		Mode:            in.Mode,
		DurationMinutes: in.DurationMinutes,
	}

	errf := func(code, field, format string, args ...any) {
		report.Problems = append(report.Problems, Problem{
			Code:     code,
			Severity: SeverityError,
			Field:    field,
			Detail:   fmt.Sprintf(format, args...),
		})
	}
	warnf := func(code, field, format string, args ...any) {
		report.Problems = append(report.Problems, Problem{
			Code:     code,
			Severity: SeverityWarning,
			Field:    field,
			Detail:   fmt.Sprintf(format, args...),
		})
	}

	modeKnown := in.Mode == AgendaMode || in.Mode == MinutesMode
	if !modeKnown {
		errf(CodeInputModeInvalid, "mode", "mode %q is neither %q nor %q", string(in.Mode), AgendaMode, MinutesMode)
	} else if out.Mode != in.Mode {
		errf(CodeOutputModeMismatch, "mode", "the proposal answers mode %q but %q was requested", string(out.Mode), string(in.Mode))
	}

	// The requested mode decides which fields are legal: an agenda has time
	// boxes, minutes have decisions, actions and open questions, never both.
	switch in.Mode {
	case AgendaMode:
		if len(out.Decisions) > 0 || len(out.Actions) > 0 || len(out.OpenQuestions) > 0 {
			errf(CodeModeFieldNotAllowed, "decisions,actions,openQuestions",
				"agenda mode may only produce topics, but the proposal carries %d decisions, %d actions and %d open questions",
				len(out.Decisions), len(out.Actions), len(out.OpenQuestions))
		}
	case MinutesMode:
		if len(out.Topics) > 0 {
			errf(CodeModeFieldNotAllowed, "topics",
				"minutes mode may only produce decisions, actions and open questions, but the proposal carries %d topics",
				len(out.Topics))
		}
	}

	checkEvidence := func(field string, evidence []string) {
		if len(in.Transcript) == 0 {
			// The missing transcript is already reported; resolving evidence ids
			// against nothing would only add noise.
			return
		}
		if len(evidence) == 0 {
			errf(CodeEvidenceMissing, field+".evidence", "the item cites no transcript segment")
			return
		}
		for _, id := range evidence {
			segment, found := in.EvidenceSegment(id)
			if !found {
				errf(CodeEvidenceUnknown, field+".evidence", "evidence id %q is not a segment of the supplied transcript", id)
				continue
			}
			if strings.TrimSpace(segment.Timestamp) == "" {
				warnf(CodeEvidenceNoTimestamp, field+".evidence",
					"segment %q has no timestamp, so the minute keeps the quote without a time", id)
			}
		}
	}

	if in.Mode == AgendaMode {
		if len(in.Goals) == 0 {
			errf(CodeEmptyGoals, "goals", "an agenda needs at least one goal")
		}
		if in.DurationMinutes <= 0 {
			errf(CodeMissingDuration, "durationMinutes", "an agenda needs the requested meeting duration in minutes")
		}
		if len(out.Topics) == 0 {
			errf(CodeNoTopics, "topics", "an agenda needs at least one time-boxed topic")
		}

		seen := make(map[string]int, len(out.Topics))
		for i, topic := range out.Topics {
			field := fmt.Sprintf("topics[%d]", i)
			report.TopicMinutes += topic.Minutes

			if topic.Minutes <= 0 {
				errf(CodeTopicMinutesInvalid, field+".minutes",
					"topic %q needs a positive time box, got %d minutes", topic.Title, topic.Minutes)
			}

			key := normalizeLabel(topic.Title)
			if key == "" {
				errf(CodeEmptyStatement, field+".title", "topic %d has no title", i)
			} else if first, repeated := seen[key]; repeated {
				errf(CodeDuplicateTopic, field+".title", "topic %q repeats the topic at index %d", topic.Title, first)
			} else {
				seen[key] = i
			}

			if len(in.Goals) > 0 && (topic.GoalIndex < 0 || topic.GoalIndex >= len(in.Goals)) {
				errf(CodeGoalIndexOutOfRange, field+".goalIndex",
					"topic %q points at goal %d, but %d goals were supplied", topic.Title, topic.GoalIndex, len(in.Goals))
			}

			if isUnknownOwner(topic.Owner) {
				report.RequiresConfirmation = true
				warnf(CodeOwnerNeedsConfirmation, field+".owner",
					"topic %q has no stated owner, so a human has to assign one", topic.Title)
			}
		}

		if in.DurationMinutes > 0 && report.TopicMinutes > in.DurationMinutes {
			errf(CodeDurationOverflow, "topics",
				"the time boxes total %d minutes, which does not fit the requested %d minutes",
				report.TopicMinutes, in.DurationMinutes)
		}
	}

	if in.Mode == MinutesMode {
		if len(in.Transcript) == 0 {
			errf(CodeEmptyTranscript, "transcript", "minutes need the transcript segments they cite")
		}
		if len(out.Decisions) == 0 && len(out.Actions) == 0 && len(out.OpenQuestions) == 0 {
			warnf(CodeMinutesNoItems, "decisions,actions,openQuestions",
				"the transcript produced no decision, action or open question")
		}

		for i, decision := range out.Decisions {
			field := fmt.Sprintf("decisions[%d]", i)
			if strings.TrimSpace(decision.Statement) == "" {
				errf(CodeEmptyStatement, field+".statement", "decision %d has no statement", i)
			}
			checkEvidence(field, decision.Evidence)
		}

		for i, action := range out.Actions {
			field := fmt.Sprintf("actions[%d]", i)
			if strings.TrimSpace(action.Task) == "" {
				errf(CodeEmptyStatement, field+".task", "action %d has no task", i)
			}
			checkEvidence(field, action.Evidence)
			if isUnknownOwner(action.Owner) {
				report.RequiresConfirmation = true
				warnf(CodeOwnerNeedsConfirmation, field+".owner",
					"action %q has no stated owner, so a human has to assign one", action.Task)
			}
			if isUnknownDate(action.DueDate) {
				report.RequiresConfirmation = true
				warnf(CodeDueDateNeedsConfirmation, field+".dueDate",
					"action %q has no stated due date, so a human has to set one", action.Task)
			}
		}

		for i, question := range out.OpenQuestions {
			field := fmt.Sprintf("openQuestions[%d]", i)
			if strings.TrimSpace(question.Question) == "" {
				errf(CodeEmptyStatement, field+".question", "open question %d has no text", i)
			}
			checkEvidence(field, question.Evidence)
		}
	}

	return report
}

// normalizeLabel folds case and collapses whitespace so that two spellings of
// the same label are recognized as one.
func normalizeLabel(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func isUnknownOwner(owner string) bool {
	trimmed := strings.TrimSpace(owner)
	return trimmed == "" || strings.EqualFold(trimmed, UnknownOwner)
}

func isUnknownDate(date string) bool {
	trimmed := strings.TrimSpace(date)
	return trimmed == "" || strings.EqualFold(trimmed, UnknownDate)
}
