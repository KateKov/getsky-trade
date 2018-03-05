package httputil

import (
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
)

// StatusError represents http handler error, includes original error and http status code to return
type StatusError struct {
	Err  error
	Code int
}

// Implement error interface
func (se StatusError) Error() string {
	return se.Err.Error()
}

// APIHandler is a custom hadler function used internally to define api endpoint handlers
type APIHandler func(w http.ResponseWriter, r *http.Request) error

// ValidateContentType validates request's Content-Type and returns error if it does not match expected value
func ValidateContentType(r *http.Request, expectedType string) error {
	if r.Header.Get("Content-Type") != expectedType {
		return StatusError{
			Err:  fmt.Errorf("Invalid content type, expected %s", expectedType),
			Code: http.StatusUnsupportedMediaType,
		}
	}
	return nil
}

// ErrorHandler wraps APIHandler and converts it to http.Handler by handling any returned error
func ErrorHandler(log logrus.FieldLogger, h APIHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err != nil {
			switch e := err.(type) {
			case StatusError:
				log.Errorf("%s: HTTP %d - %s", r.URL, e.Code, e)
				http.Error(w, e.Error(), e.Code)
			default:
				log.Errorf("Error in handler %s - %s", r.URL, err)
				http.Error(w, http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError)
			}
		}
	}
}

// JSONHandler wraps Handler and adds json content type
func JSONHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}
}