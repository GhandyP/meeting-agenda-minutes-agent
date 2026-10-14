// Package minutes renders deterministic, unpublished minutes artifacts.
package minutes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/approval"
	"github.com/GhandyP/meeting-agenda-minutes-agent/internal/meeting"
)

// ErrEvidenceNotFound is returned when a minutes item cites an id absent from
// the supplied transcript.
var ErrEvidenceNotFound = errors.New("evidence segment not found")

// Document renders minutes as deterministic Markdown, retaining each cited
// transcript segment as its speaker, supplied timestamp, and quote.
func Document(in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) ([]byte, error) {
	if in.Mode != meeting.MinutesMode {
		return nil, fmt.Errorf("minutes document requires minutes input mode, got %q", in.Mode)
	}
	if out.Mode != in.Mode {
		return nil, fmt.Errorf("minutes input/output mode mismatch: %q and %q", in.Mode, out.Mode)
	}

	title := out.Title
	if title == "" {
		title = in.Title
	}
	if title == "" {
		title = "Meeting minutes"
	}
	var document strings.Builder
	fmt.Fprintf(&document, "# %s\n\n", title)
	document.WriteString("> **DRY RUN — these minutes are not published.**\n\n")

	document.WriteString("## Decisions\n\n")
	if len(out.Decisions) == 0 {
		document.WriteString("No decisions recorded.\n\n")
	}
	for i, decision := range out.Decisions {
		fmt.Fprintf(&document, "%d. %s\n", i+1, decision.Statement)
		if err := writeEvidence(&document, in, decision.Evidence); err != nil {
			return nil, err
		}
		document.WriteByte('\n')
	}

	document.WriteString("## Actions\n\n")
	if len(out.Actions) == 0 {
		document.WriteString("No actions recorded.\n\n")
	}
	for i, action := range out.Actions {
		owner := action.Owner
		if owner == "" {
			owner = "not specified"
		}
		dueDate := action.DueDate
		if dueDate == "" {
			dueDate = "not specified"
		}
		fmt.Fprintf(&document, "%d. %s — Owner: %s; Due date: %s\n", i+1, action.Task, owner, dueDate)
		if err := writeEvidence(&document, in, action.Evidence); err != nil {
			return nil, err
		}
		document.WriteByte('\n')
	}

	document.WriteString("## Open questions\n\n")
	if len(out.OpenQuestions) == 0 {
		document.WriteString("No open questions recorded.\n\n")
	}
	for i, question := range out.OpenQuestions {
		fmt.Fprintf(&document, "%d. %s\n", i+1, question.Question)
		if err := writeEvidence(&document, in, question.Evidence); err != nil {
			return nil, err
		}
		document.WriteByte('\n')
	}
	return []byte(document.String()), nil
}

// WriteDryRun writes one approved, deterministic Markdown artifact under dir.
func WriteDryRun(dir string, g approval.Gate, token string, in meeting.MeetingPlanInput, out meeting.AgendaOrMinutes) (string, error) {
	if err := g.Authorize(token, in, out); err != nil {
		return "", err
	}
	name, err := approval.ArtifactFileName(in.MeetingID, "md")
	if err != nil {
		return "", err
	}
	data, err := Document(in, out)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create minutes artifact directory: %w", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write minutes artifact: %w", err)
	}
	return path, nil
}

func writeEvidence(document *strings.Builder, in meeting.MeetingPlanInput, evidenceIDs []string) error {
	if len(evidenceIDs) == 0 {
		document.WriteString("   - Evidence: none supplied\n")
		return nil
	}
	for _, id := range evidenceIDs {
		segment, ok := in.EvidenceSegment(id)
		if !ok {
			return fmt.Errorf("%w: %q", ErrEvidenceNotFound, id)
		}
		fmt.Fprintf(document, "   - Evidence id: %s\n", id)
		fmt.Fprintf(document, "     > Speaker: %s\n", segment.Speaker)
		if segment.Timestamp != "" {
			fmt.Fprintf(document, "     > Timestamp: %s\n", segment.Timestamp)
		}
		for _, line := range strings.Split(segment.Text, "\n") {
			fmt.Fprintf(document, "     > Quote: %s\n", line)
		}
	}
	return nil
}
