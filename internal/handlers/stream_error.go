package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// errorTypeForStatus names the cause in the taxonomy this API documents. Every
// error event used to say "api_error" whatever had happened, so a client that
// branches on the type - rate_limit_error in particular - could not tell a
// rate limit from a server fault. The status code was the only clue and it is a
// nonstandard extension field.
func errorTypeForStatus(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "rate_limit_error"
	case status == http.StatusServiceUnavailable:
		return "overloaded_error"
	case status == http.StatusBadRequest, status == http.StatusRequestEntityTooLarge, status == http.StatusUnprocessableEntity:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

func streamErrorDetails(message string, cause error) types.APIError {
	return settleErrorType(streamErrorCause(message, cause))
}

// settleErrorType names the cause once its status is known.
func settleErrorType(details types.APIError) types.APIError {
	details.Type = errorTypeForStatus(details.StatusCode)
	return details
}

func streamErrorCause(message string, cause error) types.APIError {
	details := types.APIError{Message: message}
	var cooling *core.KeysCoolingError
	if errors.As(cause, &cooling) {
		details.StatusCode = http.StatusServiceUnavailable
		details.RetryAt = cooling.RetryAt.UTC().Format(time.RFC3339Nano)
		remaining := time.Until(cooling.RetryAt)
		seconds := max(int64(1), int64(remaining/time.Second))
		if remaining > time.Second && remaining%time.Second != 0 {
			seconds++
		}
		details.RetryAfter = strconv.FormatInt(seconds, 10)
		return details
	}
	var apiErr *client.APIError
	if errors.As(cause, &apiErr) {
		details.StatusCode = apiErr.StatusCode
		if apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode == http.StatusServiceUnavailable {
			details.RetryAfter = apiErr.RetryAfter
		}
	}
	return details
}
