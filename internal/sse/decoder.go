package sse

import (
	"bytes"
	"fmt"
	"io"
)

// Decoder dispatches complete events only; Read boundaries are not event boundaries.
// It retains at most 1 MiB per event and does not own the input stream.
type Decoder struct {
	line, data      []byte
	event           string
	started, skipLF bool
}

func (d *Decoder) Feed(chunk []byte, dispatch func(string, []byte) error) error {
	for _, c := range chunk {
		if d.skipLF && c == '\n' {
			d.skipLF = false
			continue
		}
		d.skipLF = c == '\r'
		if len(d.line)+len(d.data)+len(d.event) >= 1<<20 {
			return fmt.Errorf("upstream SSE event exceeds 1 MiB")
		}
		if c != '\n' && c != '\r' {
			d.line = append(d.line, c)
			continue
		}
		line := d.line
		if !d.started {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			d.started = true
		}
		if len(line) == 0 {
			if len(d.data) > 0 {
				if err := dispatch(d.event, d.data[:len(d.data)-1]); err != nil {
					return err
				}
			}
			d.data = d.data[:0]
			d.event = ""
		} else {
			field, value, _ := bytes.Cut(line, []byte(":"))
			switch string(field) {
			case "event":
				d.event = string(bytes.TrimPrefix(value, []byte(" ")))
			case "data":
				d.data = append(d.data, bytes.TrimPrefix(value, []byte(" "))...)
				d.data = append(d.data, '\n')
			}
		}
		d.line = d.line[:0]
	}
	return nil
}

// Finish rejects an incomplete frame; EOF never dispatches buffered data.
func (d *Decoder) Finish() error {
	if len(d.data) > 0 || len(d.line) > 0 {
		return fmt.Errorf("upstream SSE ended within an event: %w", io.ErrUnexpectedEOF)
	}
	return nil
}
