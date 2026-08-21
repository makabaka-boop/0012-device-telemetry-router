package httpapi

import (
	"context"
	"net/http"

	"device-telemetry-router/internal/auth"
	"device-telemetry-router/internal/service"
)

// operatorContext injects the authenticated operator (if any) from the
// request into the service context so write operations are attributable.
func operatorContext(r *http.Request) context.Context {
	ctx := r.Context()
	if op, ok := auth.OperatorFromContext(ctx); ok {
		return service.WithOperator(ctx, op)
	}
	return ctx
}
