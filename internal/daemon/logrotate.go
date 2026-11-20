package daemon

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"
)

// logFileMax bounds the daemon log. Background mode appends to one file for the
// life of the machine: at 10k requests/day it grew by roughly 800 MB over 90
// days, with no rotation, so the only thing that ever reclaimed it was deleting
// the file by hand.
const logFileMax = 16 << 20

// logRotateCheck is how often the writer test the size. The check is one stat
// per log call, so it is throttled rather than done on every write.
const logRotateCheck = 60 * time.Second

// RotatingWriter appends to path and renames it to path+".1" whenever it grows
// past limit. One previous generation is kept: the point is a bounded file, not
// an archive, and the journal on Linux already keeps what happened.
//
// It is deliberately small. slog/lumberjack would add a dependency for the
// three things this does not need (compression, age pruning, backoff).
type RotatingWriter struct {
	path  string
	limit int64
	file  *os.File
	size  int64
	next  time.Time
}

// NewRotatingWriter opens path for appending. A file that is already over the
// limit is rotated on the first write, so enabling this on an existing install
// bounds the log immediately instead of after it doubles.
func NewRotatingWriter(path string, limit int64) (*RotatingWriter, error) {
	w := &RotatingWriter{path: path, limit: limit}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *RotatingWriter) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot stat log file: %w", err)
	}
	w.file, w.size = f, info.Size()
	return nil
}

// Write appends one log line. After a failed reopen it drops output rather than
// nil-panicking: the process keeps serving, and the loss was announced once at
// the moment it started.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	if w.file == nil {
		return len(p), nil
	}
	now := time.Now()
	if w.size > 0 && (now.After(w.next) || w.size+int64(len(p)) > w.limit) {
		w.next = now.Add(logRotateCheck)
		if st, err := w.file.Stat(); err == nil {
			w.size = st.Size()
		}
		if w.size+int64(len(p)) > w.limit {
			w.rotate()
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// rotate renames the current file aside and opens a fresh one. A rename that
// fails leaves the append in place: an oversized log is a nuisance, a silent
// gap in it is worse, and the error goes to the default handler since the
// logger this writer backs is what would have carried it.
func (w *RotatingWriter) rotate() {
	_ = w.file.Close()
	if err := os.Rename(w.path, w.path+".1"); err != nil {
		slog.Warn("log rotation failed; continuing to append", "path", w.path, "error", err)
	}
	if err := w.open(); err != nil {
		slog.Error("log rotation could not reopen the log file", "path", w.path, "error", err)
		w.file = nil
	}
}

// Close flushes and closes the file.
func (w *RotatingWriter) Close() error {
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

var _ io.WriteCloser = (*RotatingWriter)(nil)
