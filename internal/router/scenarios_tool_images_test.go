package router

import "testing"

func TestRequestFactsRecognizeImagesAcrossLatestUserTurn(t *testing.T) {
	image := MessageContent{Role: "user", HasImage: true, ImageHashes: []string{"shot"}}
	text := MessageContent{Role: "user", Content: "tool completed"}
	assistant := MessageContent{Role: "assistant", Content: "calling tools"}
	for _, tc := range []struct {
		name     string
		messages []MessageContent
		want     bool
	}{
		{"split tool results", []MessageContent{assistant, image, text}, true},
		{"image twice within turn", []MessageContent{assistant, image, image, text}, true},
		{"historical image", []MessageContent{image, assistant, text}, false},
		{"repeated historical image", []MessageContent{image, assistant, image, text}, false},
		{"image with no hash", []MessageContent{assistant, {Role: "user", HasImage: true}, text}, true},
		{"plain text turn", []MessageContent{assistant, text, text}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := AnalyzeRequestFacts(tc.messages)
			if facts.NeedsVision != tc.want || facts.LatestUserHasImage != tc.want || facts.LatestUserText != text.Content {
				t.Fatalf("got %+v; want vision=%t with latest text unchanged", facts, tc.want)
			}
		})
	}
}
