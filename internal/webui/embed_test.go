package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerServesSomethingAtRoot(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	// Whether the SPA is built (index.html) or not (placeholder), root returns 200.
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandlerServesIndexForUnknownClientRoute(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	// A non-asset path (client-side route) should fall back to 200, not 404.
	resp, err := http.Get(srv.URL + "/tickets/abc")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
