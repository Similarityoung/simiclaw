package routing

import (
	"context"
	"errors"

	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
	runtimepayload "github.com/similarityyoung/simiclaw/internal/runtime/payload"
)

type Service struct {
	payloads *runtimepayload.Registry
}

func NewService(payloads *runtimepayload.Registry) *Service {
	return &Service{payloads: payloads}
}

func (s *Service) Resolve(_ context.Context, in gatewaymodel.NormalizedIngress, binding gatewaymodel.BindingContext) (gatewaymodel.RouteDecision, error) {
	if s.payloads == nil {
		return gatewaymodel.RouteDecision{}, errors.New("payload registry unavailable")
	}
	payloadType := payloadTypeFor(in, binding)
	plan := s.payloads.Resolve(payloadType)
	return gatewaymodel.RouteDecision{
		RunMode:        plan.RunMode,
		PayloadType:    payloadType,
		SuppressOutput: plan.SuppressOutput,
		Metadata:       cloneMetadata(binding.Metadata),
	}, nil
}
