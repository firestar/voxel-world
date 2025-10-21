package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSONHandlesEncodingFailures(t *testing.T) {
	recorder := httptest.NewRecorder()

	// encoding/json cannot marshal channel values and returns an error.
	data := struct{ C chan int }{C: make(chan int)}
	writeJSON(recorder, data)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestWriteJSONSetsContentType(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, struct{ Value string }{Value: "ok"})

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}
