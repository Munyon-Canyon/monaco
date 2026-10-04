package testkit

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
)

func Serves(mount func(api.Mount), method, target string) bool {
	mux := http.NewServeMux()
	mount(api.Mount{Mux: mux})
	_, pattern := mux.Handler(httptest.NewRequestWithContext(context.Background(), method, target, nil))
	return pattern != ""
}
