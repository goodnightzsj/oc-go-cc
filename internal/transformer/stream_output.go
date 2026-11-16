package transformer

import "io"

// Tag errors at the actual output boundary, before an adapter can join them
// with a source failure while emitting response.failed.
type streamOutputError struct{ error }

func (e *streamOutputError) Unwrap() error { return e.error }

type streamOutputWriter struct{ io.Writer }

func (w streamOutputWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		err = &streamOutputError{err}
	}
	return n, err
}
