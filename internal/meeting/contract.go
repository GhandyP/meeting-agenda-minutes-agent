// Package meeting holds the typed meeting-agenda/minutes contract and the
// deterministic rules a proposal must satisfy before a human approves it.
//
// The package has no dependency on ADK, on a model, or on the network: the same
// rules run in tests, in the web app, and in the headless command.
package meeting

// Mode selects which half of the contract a meeting uses: the time-boxed agenda
// of a meeting that has not happened yet, or the minutes of one that did.
type Mode string

const (
	// AgendaMode plans a future meeting with time-boxed topics.
	AgendaMode Mode = "agenda"
	// MinutesMode records a meeting that already happened.
	MinutesMode Mode = "minutes"
)

// UnknownOwner is the explicit placeholder for an owner the source text never
// named. The rules never infer an owner from context, so this value is legal but
// always needs human confirmation.
const UnknownOwner = "unknown"

// UnknownDate is the explicit placeholder for a due date the source text never
// stated.
const UnknownDate = "unknown"

// Segment is one span of a supplied transcript. Timestamps are treated as
// supplied strings and are never re-derived or re-formatted.
type Segment struct {
	ID        string `json:"id"`
	Speaker   string `json:"speaker,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Text      string `json:"text"`
}

// MeetingPlanInput is the caller-supplied request. Goals drive an agenda;
// Transcript drives minutes. The two are not interchangeable.
type MeetingPlanInput struct {
	MeetingID       string    `json:"meetingId"`
	Mode            Mode      `json:"mode"`
	Title           string    `json:"title,omitempty"`
	DurationMinutes int       `json:"durationMinutes,omitempty"`
	Participants    []string  `json:"participants,omitempty"`
	Goals           []string  `json:"goals,omitempty"`
	Transcript      []Segment `json:"transcript,omitempty"`
}

// Topic is one time-boxed agenda item.
type Topic struct {
	Title     string `json:"title"`
	Owner     string `json:"owner"`
	Minutes   int    `json:"minutes"`
	GoalIndex int    `json:"goalIndex"`
}

// Decision is one settled item of a meeting that already happened.
type Decision struct {
	Statement string   `json:"statement"`
	Evidence  []string `json:"evidence"`
}

// Action is one follow-up with an explicit owner, an optional due date, and the
// transcript segments that support it.
type Action struct {
	Task     string   `json:"task"`
	Owner    string   `json:"owner"`
	DueDate  string   `json:"dueDate,omitempty"`
	Evidence []string `json:"evidence"`
}

// OpenQuestion is one item the meeting left unresolved.
type OpenQuestion struct {
	Question string   `json:"question"`
	Evidence []string `json:"evidence"`
}

// AgendaOrMinutes is the single output shape of both modes. Agenda mode fills
// Topics only; minutes mode fills Decisions, Actions and OpenQuestions only.
type AgendaOrMinutes struct {
	Mode          Mode           `json:"mode"`
	Title         string         `json:"title"`
	Topics        []Topic        `json:"topics,omitempty"`
	Decisions     []Decision     `json:"decisions,omitempty"`
	Actions       []Action       `json:"actions,omitempty"`
	OpenQuestions []OpenQuestion `json:"openQuestions,omitempty"`
}

// EvidenceSegment resolves an evidence id against the supplied transcript.
func (in MeetingPlanInput) EvidenceSegment(id string) (Segment, bool) {
	for _, segment := range in.Transcript {
		if segment.ID == id {
			return segment, true
		}
	}
	return Segment{}, false
}
