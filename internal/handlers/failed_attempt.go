package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/pkg/types"
)

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
	cause := err
	var source *core.ResponseError
	if errors.As(err, &source) {
		cause = source.Err // A joined delivery cancellation does not erase the source failure.
	}
	if ctx.Err() != nil && reported == nil && errors.Is(cause, ctx.Err()) {
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
	rec.Duration = time.Since(rec.StartTime)
	if err := h.storage.InsertRequest(rec); err != nil {
		h.logger.Warn("failed to insert failed attempt into storage", "error", err)
	}
}
