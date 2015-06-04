package useful

import (
	"net/http"

	ch "github.com/EricLagerg/compressedhandler"
)

// Handler ia a wrapper for an http.Handler with an io.Writer for
// writing to log files.
type Handler struct {
	handler http.Handler
}

// NewUsefulHandler returns a *Handler with logging capabilities as well
// as potentially compressed content.
func NewUsefulHandler(handler http.Handler) http.Handler {
	setWriter()

	return &Handler{
		handler: ch.CompressedHandler(handler),
	}
}
