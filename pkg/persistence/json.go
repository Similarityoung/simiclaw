package persistence

import (
	"bytes"
	"encoding/json"
)

func decodeJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(dst)
}
