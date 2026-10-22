package provider

import (
	"bytes"
	"io"
	"time"
)

// peekedBody replays the bytes a peek consumed before reading further.
//
// The stream contract is "forward what the upstream sent, unmodified". Peeking
// for the model would violate it by swallowing the frame it read, so the same
// bytes are handed back on the next Read and the caller never learns a peek
// happened.
type peekedBody struct {
	io.ReadCloser
	pending []byte
}

func (p *peekedBody) Read(b []byte) (int, error) {
	if len(p.pending) > 0 {
		n := copy(b, p.pending)
		p.pending = p.pending[n:]
		return n, nil
	}
	return p.ReadCloser.Read(b)
}

// The peek wrapper sits between the observer and everything downstream, so it
// has to forward every observation the observer publishes. Embedding promotes
// Read and Close only: a wrapper that forwards one and not the others erases
// them, which is the failure this project has already hit twice.
func (p *peekedBody) ServedModel() string {
	if reporter, ok := p.ReadCloser.(interface{ ServedModel() string }); ok {
		return reporter.ServedModel()
	}
	return ""
}

func (p *peekedBody) UpstreamChannel() string {
	if reporter, ok := p.ReadCloser.(interface{ UpstreamChannel() string }); ok {
		return reporter.UpstreamChannel()
	}
	return ""
}

func (p *peekedBody) MarkFirstByte() {
	if recorder, ok := p.ReadCloser.(interface{ MarkFirstByte() }); ok {
		recorder.MarkFirstByte()
	}
}

func (p *peekedBody) FirstByteAt() time.Time {
	if reporter, ok := p.ReadCloser.(interface{ FirstByteAt() time.Time }); ok {
		return reporter.FirstByteAt()
	}
	return time.Time{}
}

func (p *peekedBody) UpstreamModel() string {
	if reporter, ok := p.ReadCloser.(interface{ UpstreamModel() string }); ok {
		return reporter.UpstreamModel()
	}
	return ""
}

// peekServedModel reads ahead until the upstream names the model, then returns a
// body that replays everything it consumed.
//
// It stops at the first frame carrying a model, which on this gateway is the
// first frame - so the cost is the wait for the upstream's first byte, which
// message_start already had to wait for. It does not block on a missing model:
// after maxPeekBytes it gives up and returns an empty value, because a stream
// that never names its model must still be forwarded rather than held open.
//
// body must be the observer wrapper: the peek reads through it, so the observer
// sees the frames it consumes and answers without a second parser here.
func peekServedModel(body io.ReadCloser, maxPeekBytes int) (io.ReadCloser, string, bool) {
	reporter, ok := body.(interface{ ServedModel() string })
	if !ok || body == nil {
		return body, "", false
	}
	if model := reporter.ServedModel(); model != "" {
		// Already drained by an earlier peek: the first byte is behind us, and
		// reporting it again would claim a second one arrived.
		return body, model, false
	}
	if maxPeekBytes <= 0 {
		return body, "", false
	}

	// Read in small chunks rather than by line: the observer already splits
	// lines internally, so anything it can answer has arrived once Read
	// returns, and a chunk boundary here does not have to line up with a frame.
	var consumed bytes.Buffer
	buf := make([]byte, 4096)
	for consumed.Len() < maxPeekBytes {
		n, err := body.Read(buf)
		if n > 0 {
			consumed.Write(buf[:n])
			if model := reporter.ServedModel(); model != "" {
				return &peekedBody{ReadCloser: body, pending: consumed.Bytes()}, model, true
			}
		}
		if err != nil {
			break
		}
	}
	// The peek read at least one chunk before giving up, so a first byte did
	// arrive even though it never named a model.
	return &peekedBody{ReadCloser: body, pending: consumed.Bytes()}, "", consumed.Len() > 0
}

// maxServedModelPeekBytes bounds the read-ahead. The gateway names the model in
// its first frame, so this is generous slack for a slow or chatty edge; past it
// the stream is forwarded with whatever model was seen, which is the honest
// answer rather than holding the response open.
const maxServedModelPeekBytes = 64 << 10
