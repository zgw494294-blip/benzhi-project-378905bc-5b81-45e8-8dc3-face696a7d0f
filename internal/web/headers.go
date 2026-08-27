package web

import "net/http"

// WriteJSONHeaders applies the response metadata shared by API endpoints.
func WriteJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
}

// MethodNotAllowed keeps method errors consistent for auxiliary handlers.
func MethodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
