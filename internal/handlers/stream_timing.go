package handlers

import (
	"encoding/json"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// streamTiming observes the application output boundary, not client receipt.
type streamTiming struct {
	firstEvent   time.Time
	firstContent time.Time
	messageEnd   time.Time
}

type streamMilestones struct{ event, content, end bool }

func (t streamTiming) applyTo(rec *history.RequestRecord) {
	rec.FirstEventMs = observedOffsetMillis(t.firstEvent, rec.StartTime)
	rec.FirstContentMs = observedOffsetMillis(t.firstContent, rec.StartTime)
	rec.MessageEndMs = observedOffsetMillis(t.messageEnd, rec.StartTime)
}

func observedOffsetMillis(at, start time.Time) *int64 {
	if at.IsZero() || start.IsZero() || at.Before(start) {
		return nil
	}
	ms := at.Sub(start).Milliseconds()
	return &ms
}

func (t *streamTiming) record(seen streamMilestones) {
	event := seen.event && t.firstEvent.IsZero()
	content := seen.content && t.firstContent.IsZero()
	end := seen.end && t.messageEnd.IsZero()
	if !event && !content && !end {
		return
	}
	at := time.Now()
	if event {
		t.firstEvent = at
	}
	if content {
		t.firstContent = at
	}
	if end {
		t.messageEnd = at
	}
}

func hasTimingContent(data []byte, kind string) bool {
	if kind != "content_block_start" && kind != "content_block_delta" {
		return false
	}
	var event struct {
		ContentBlock struct{ Type, Text, Thinking, ID, Name string } `json:"content_block"`
		Delta        struct{ Type, Text, Thinking string }           `json:"delta"`
	}
	// Optional timing metadata must not change the existing forwarding contract.
	if json.Unmarshal(data, &event) != nil {
		return false
	}
	if kind == "content_block_start" {
		b := event.ContentBlock
		switch b.Type {
		case "text":
			return b.Text != ""
		case "thinking":
			return b.Thinking != ""
		case "tool_use":
			return b.ID != "" && b.Name != ""
		}
	} else {
		switch event.Delta.Type {
		case "text_delta":
			return event.Delta.Text != ""
		case "thinking_delta":
			return event.Delta.Thinking != ""
		}
	}
	return false
}
