package relay

import (
	"net/http"
	"testing"
)

func TestIsHTTPSuccessStatusAcceptsAny2xx(t *testing.T) {
	for _, statusCode := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent} {
		if !isHTTPSuccessStatus(statusCode) {
			t.Fatalf("isHTTPSuccessStatus(%d) = false, want true", statusCode)
		}
	}
	for _, statusCode := range []int{http.StatusContinue, http.StatusMultipleChoices, http.StatusBadRequest, http.StatusInternalServerError} {
		if isHTTPSuccessStatus(statusCode) {
			t.Fatalf("isHTTPSuccessStatus(%d) = true, want false", statusCode)
		}
	}
}
