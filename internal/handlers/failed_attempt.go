package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

var errAttemptTimeout = fmt.Errorf("upstream attempt timeout: %w", context.DeadlineExceeded)

// Keep the attempt's own deadline distinct from a later caller deadline.
func attemptFailure(ctx context.Context, err error) error {
	cause := context.Cause(ctx)
	if !errors.Is(cause, errAttemptTimeout) {
		return err
	}
	original := err
	var source *core.ResponseError
	if errors.As(err, &source) {
		original = source.Err
	}
	if errors.Is(original, context.DeadlineExceeded) || errors.Is(original, context.Canceled) ||
		errors.Is(original, transformer.ErrStreamReadCanceled) || errors.Is(original, transformer.ErrClientDisconnected) {
		return errors.Join(cause, err)
	}
	return err
}

// A refusal before a response body opens is still an observed failed attempt.
func (h *MessagesHandler) recordFailedAttempt(ctx context.Context, rec history.RequestRecord, err error) {
	if h.storage == nil {
		return
	}
	var failure interface{ ReportedUsage() *types.Usage }
	var reported *types.Usage
	if errors.As(err, &failure) {
		reported = failure.ReportedUsage()
	}
	if reported == nil && isCallerCancellation(ctx, err) {
		return // A caller that stopped waiting is not an observed source failure.
	}
	if reported != nil {
		rec.InputTokens, rec.OutputTokens = reported.InputTokens, reported.OutputTokens
		rec.CacheReadTokens, rec.CacheCreationTokens = reported.CacheReadInputTokens, reported.CacheCreationInputTokens
		rec.ReasoningTokens = reported.ReasoningTokens
	}
	rec.ID = uuid.NewString()
	rec.Success, rec.ErrorMsg = false, err.Error()
	rec.UsageUnobserved = reported == nil
	// The scheduler stamps the credential label on API errors when the
	// attempt settles, so the row says which key was rejected - the one fact
	// the fallback logs cannot show for the attempt that never opened a body.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		rec.KeyHint = apiErr.KeyHint
	}
	rec.Duration = time.Since(rec.StartTime)
	if err := h.storage.InsertRequest(rec); err != nil {
		h.logger.Warn("failed to insert failed attempt into storage", "error", err)
	}
}

func isCallerCancellation(ctx context.Context, err error) bool {
	if ctx.Err() == nil || errors.Is(err, errAttemptTimeout) {
		return false
	}
	var source *core.ResponseError
	if errors.As(err, &source) {
		err = source.Err // A joined delivery cancellation does not erase the source failure.
	}
	return errors.Is(err, ctx.Err()) || errors.Is(err, transformer.ErrClientDisconnected) || errors.Is(err, transformer.ErrStreamReadCanceled)
}
