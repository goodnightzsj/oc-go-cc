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

func streamErrorDetails(message string, cause error) types.APIError {
	details := types.APIError{Type: "api_error", Message: message}
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
