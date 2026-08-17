// Package token provides token counting utilities using tiktoken encoding.
package token

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkoukk/tiktoken-go"
)

// Counter handles token counting for text and message arrays.
type Counter struct {
	tiktoken *tiktoken.Tiktoken
}

// defaultCacheDir returns a user-writable cache directory for tiktoken files.
// Uses TIKTOKEN_CACHE_DIR or DATA_GYM_CACHE_DIR if already set; otherwise
// defaults to ~/.cache/routatic-proxy/tiktoken to avoid /tmp permission issues.
func defaultCacheDir() string {
	if d := os.Getenv("TIKTOKEN_CACHE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("DATA_GYM_CACHE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "data-gym-cache")
	}
	return filepath.Join(home, ".cache", "routatic-proxy", "tiktoken")
}

// NewCounter creates a new token counter with cl100k_base encoding.
func NewCounter() (*Counter, error) {
	// Set process-wide env var before tiktoken loads any encoding files.
	// This is safe because NewCounter is called once at startup.
	_ = os.Setenv("TIKTOKEN_CACHE_DIR", defaultCacheDir())

	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("failed to get encoding: %w", err)
	}
	return &Counter{tiktoken: enc}, nil
}

// CountTokens counts tokens in a string. tiktoken.Encode cannot fail, so there
// is no error to report.
func (c *Counter) CountTokens(text string) int {
	return len(c.tiktoken.Encode(text, nil, nil))
}

// MessageContent represents a single message in a conversation.
type MessageContent struct {
	Role        string
	Content     string
	ExtraTokens int
}

// CountMessages counts tokens in a message array.
// Estimates tokens for system prompt + messages with formatting overhead.
func (c *Counter) CountMessages(system string, messages []MessageContent) int {
	total := 3 // Start token

	if system != "" {
		total += c.CountTokens(system) + 5 // System prompt overhead
	}

	for _, msg := range messages {
		total += c.CountTokens(msg.Content) + 5 // Per-message overhead
		total += msg.ExtraTokens
	}

	return total
}
