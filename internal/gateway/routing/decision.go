package routing

import (
	gatewaybindings "github.com/similarityyoung/simiclaw/internal/gateway/bindings"
	gatewaymodel "github.com/similarityyoung/simiclaw/internal/gateway/model"
)

func payloadTypeFor(in gatewaymodel.NormalizedIngress, binding gatewaymodel.BindingContext) string {
	if override := gatewaybindings.PayloadTypeOverride(binding.Metadata); override != "" {
		return override
	}
	return in.Payload.Type
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
