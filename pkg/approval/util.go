package approval

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func decodeJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(dst)
}

func decodePatchPayload(payload map[string]any) (model.PatchPayload, error) {
	if payload == nil {
		return model.PatchPayload{}, fmt.Errorf("patch payload is empty")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return model.PatchPayload{}, err
	}
	var out model.PatchPayload
	if err := json.Unmarshal(b, &out); err != nil {
		return model.PatchPayload{}, err
	}
	return out, nil
}
