package api

import "github.com/similarityyoung/simiclaw/pkg/model"

type ErrorResponse struct {
	Error model.ErrorBlock `json:"error"`
}
