package handlers

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes a JSON response to the http.ResponseWriter
func WriteJSON(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}

// WriteJSONWithStatus writes a JSON response with a specific status code
func WriteJSONWithStatus(w http.ResponseWriter, statusCode int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(data)
}

// WriteError writes an error response
func WriteError(w http.ResponseWriter, statusCode int, message string) error {
	errorResponse := ErrorResponse{
		Error: message,
		Code:  statusCode,
	}
	return WriteJSONWithStatus(w, statusCode, errorResponse)
}