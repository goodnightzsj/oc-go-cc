package transformer

import "testing"

func TestNativeResponsesRoutingRejectsUnboundState(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","input":"hi","store":true}`,
		`{"model":"m","input":"hi","background":true}`,
		`{"model":"m","input":"hi","previous_response_id":"resp_1"}`,
		`{"model":"m","input":"hi","conversation":"conv_1"}`,
		`{"model":"m","input":"hi","prompt":{"id":"pmpt_1"}}`,
		`{"model":"m","input":"hi","tools":[{"type":"file_search","vector_store_ids":["vs_1"]}]}`,
		`{"model":"m","input":"hi","tools":[{"type":"code_interpreter","container":"cntr_1"}]}`,
		`{"model":"m","input":"hi","tools":[{"type":"namespace","name":"ns","tools":[{"type":"code_interpreter","container":{"type":"auto","file_ids":["file_1"]}}]}]}`,
		`{"model":"m","input":[{"type":"item_reference","id":"item_1"}]}`,
		`{"model":"m","input":[{"type":"file_search_call","id":"f_1"}]}`,
		`{"model":"m","input":[{"role":"user","content":[{"type":"input_image","file_id":"f_1"}]}]}`,
	} {
		if _, _, err := ResponsesRoutingRequest([]byte(body)); err == nil {
			t.Errorf("accepted unbound state: %s", body)
		}
	}
	if _, _, err := ResponsesRoutingRequest([]byte(`{"model":"m","input":"hi","tools":[{"type":"web_search"},{"type":"code_interpreter","container":{"type":"auto"}}]}`)); err != nil {
		t.Fatal(err)
	}
	projected, replay, err := ResponsesRoutingRequest([]byte(`{"model":"m","input":[{"type":"reasoning","id":"rs_1","encrypted_content":"synthetic","summary":[]},{"type":"custom_tool_call_output","call_id":"c_1","output":[{"type":"input_text","text":"result"},{"type":"input_image","image_url":"data:image/png;base64,aGk="}]}]}`))
	if err != nil || !replay || len(projected.Messages) != 2 {
		t.Fatalf("native replay facts lost: %+v replay=%t err=%v", projected, replay, err)
	}
}
