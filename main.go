package main

import (
	"context"
	"log"
	"os"

	"github.com/GhandyP/adk-go-enterprise-cookbook/internal/adkhelper"
)

func main() {
	if err := adkhelper.Run(
		context.Background(),
		os.Args[1:],
		"case_17_meeting_agenda_minutes_agent",
		"A text-only starting point for meeting agent: agenda and minutes.",
		"From the supplied meeting goals or transcript text, produce an agenda or minutes with decisions and actions. Do not claim to record or transcribe a meeting.",
	); err != nil {
		log.Fatal(err)
	}
}
