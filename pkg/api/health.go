package api

import "net/http"

func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	state, err := a.Supervisor.ReadyState(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, state)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
