package httpapi

import "net/http"

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	state, err := s.supervisor.ReadyState(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, state)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
