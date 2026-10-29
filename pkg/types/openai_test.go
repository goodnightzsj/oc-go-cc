package types

import (
	"encoding/json"
	"testing"
)

func TestChatMessageRefusalIsIndependentOptionalContent(t *testing.T) {
	for _, raw := range []string{
		`{"content":"text","refusal":"declined"}`,
		`{"content":"text","refusal":""}`,
		`{"content":"text","refusal":null}`,
		`{"content":"text"}`,
	} {
		var message ChatMessage
		if err := json.Unmarshal([]byte(raw), &message); err != nil {
			t.Fatal(err)
		}
		if message.ContentText() != "text" {
			t.Fatal("refusal altered ordinary content extraction")
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var output map[string]any
		if err := json.Unmarshal(encoded, &output); err != nil {
			t.Fatal(err)
		}
		refusal, exists := output["refusal"]
		if exists != (message.Refusal != nil) || exists && refusal != *message.Refusal {
			t.Fatalf("optional refusal changed: %s", encoded)
		}
	}
}
