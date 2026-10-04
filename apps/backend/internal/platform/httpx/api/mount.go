package api

import "net/http"

type Mount struct {
	Mux            *http.ServeMux
	Middlewares    []func(http.Handler) http.Handler
	InvalidRequest func(http.ResponseWriter, *http.Request, error)
	Problem        func(http.ResponseWriter, *http.Request, error)
}
