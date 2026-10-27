package sse

import (
	"errors"
	"io"
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
