package sse

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestDecoderEventFieldsAndEmptyData(t *testing.T) {
	var d Decoder
	var events []string
	for _, c := range []byte("\xef\xbb\xbfevent: error\rdata: {\rdata:  \"message\":\"synthetic\"}\r\rid: ignored\rdata\r\r") {
		if err := d.Feed([]byte{c}, func(event string, data []byte) error { events = append(events, event+":"+string(data)); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Finish(); err != nil || len(events) != 2 || events[0] != "error:{\n \"message\":\"synthetic\"}" || events[1] != ":" {
		t.Fatalf("events=%q err=%v", events, err)
	}
}

func TestDecoderRejectsOversizeAndIncompleteEvents(t *testing.T) {
	for _, raw := range []string{"data: {}", "data: {}\n"} {
		var d Decoder
		called := false
		if err := d.Feed([]byte(raw), func(string, []byte) error { called = true; return nil }); err != nil {
			t.Fatal(err)
		}
		if called || !errors.Is(d.Finish(), io.ErrUnexpectedEOF) {
			t.Fatalf("incomplete event accepted: %q", raw)
		}
	}
	var d Decoder
	if err := d.Feed([]byte("data:"+strings.Repeat("x", 1<<20)), func(string, []byte) error { return nil }); err == nil {
		t.Fatal("oversized event accepted")
	}
}

func TestDecoderPreservesConsumerError(t *testing.T) {
	var d Decoder
	want := errors.New("consumer failed")
	if err := d.Feed([]byte("data:{}\n\n"), func(string, []byte) error { return want }); !errors.Is(err, want) {
		t.Fatalf("consumer error lost: %v", err)
	}
}

func TestDecoderCommentForwarding(t *testing.T) {
	var comments []string
	d := Decoder{OnComment: func(line []byte) error {
		comments = append(comments, string(line))
		return nil
	}}
	for _, c := range []byte("\xef\xbb\xbf: first\r\n\r\n:second\r\r") {
		if err := d.Feed([]byte{c}, func(string, []byte) error {
			t.Fatal("comment dispatched as an event")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Finish(); err != nil || strings.Join(comments, "|") != ": first|:second" {
		t.Fatalf("comments=%q finish=%v", comments, err)
	}
	want := errors.New("heartbeat delivery failed")
	d.OnComment = func([]byte) error { return want }
	if err := d.Feed([]byte(":fail\n"), nil); !errors.Is(err, want) {
		t.Fatalf("comment error lost: %v", err)
	}
}

func FuzzDecoderFragmentation(f *testing.F) {
	for _, wire := range []string{
		"",
		"\xef\xbb\xbfevent: error\r\ndata: {\r\ndata: \"message\":\"test\"}\r\n\r\n",
		":heartbeat\r\rdata\r\rid: ignored\rdata: [DONE]\r\r",
		"data: incomplete\n",
	} {
		f.Add([]byte(wire), []byte{0, 1, 2, 63})
	}
	f.Fuzz(func(t *testing.T, wire, boundaries []byte) {
		if len(wire) > 8192 {
			t.Skip()
		}
		type observation struct{ kind, name, data string }
		decode := func(fragmented bool) ([]observation, error) {
			var result []observation
			d := Decoder{OnComment: func(line []byte) error {
				result = append(result, observation{kind: "comment", data: string(line)})
				return nil
			}}
			dispatch := func(name string, data []byte) error {
				result = append(result, observation{kind: "event", name: name, data: string(data)})
				return nil
			}
			for offset, part := 0, 0; offset < len(wire); part++ {
				width := len(wire) - offset
				if fragmented {
					width = 1
					if len(boundaries) > 0 {
						width += int(boundaries[part%len(boundaries)])
					}
					width = min(width, len(wire)-offset)
				}
				if err := d.Feed(wire[offset:offset+width], dispatch); err != nil {
					return result, err
				}
				offset += width
			}
			return result, d.Finish()
		}
		whole, wholeErr := decode(false)
		parts, partsErr := decode(true)
		if !slices.Equal(whole, parts) || (wholeErr == nil) != (partsErr == nil) || errors.Is(wholeErr, io.ErrUnexpectedEOF) != errors.Is(partsErr, io.ErrUnexpectedEOF) {
			t.Fatalf("fragmentation changed events or completion: whole=%q/%v parts=%q/%v", whole, wholeErr, parts, partsErr)
		}
	})
}
