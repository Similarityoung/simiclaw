package routing

import (
	"context"

	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
)

// Resolver chooses the runtime route for a normalized ingress plus binding
// context without reaching into storage details.
type Resolver interface {
	Resolve(ctx context.Context, in gatewaymodel.NormalizedIngress, binding gatewaymodel.BindingContext) (gatewaymodel.RouteDecision, error)
}
