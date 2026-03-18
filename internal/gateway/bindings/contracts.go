package bindings

import (
	"context"

	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
)

// Resolver computes binding state such as session key, scope, and hints for a
// normalized ingress request before runtime routing begins.
type Resolver interface {
	Resolve(ctx context.Context, in gatewaymodel.NormalizedIngress) (gatewaymodel.BindingContext, error)
}
