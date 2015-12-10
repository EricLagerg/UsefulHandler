package useful

import (
	"net/http"

	ch "github.com/EricLagerg/compressedhandler"
)

// Handler is a wrapper around http.Handler in order for us
// to be able to fulfill the http.Handler interface. (The interface
// requires a ServeHTTP method which we cannot provide without defining
// our own type.)
type Handler struct {
	handler http.Handler
	*Log
}

// NewUsefulHandler returns a Handler with logging capabilities as well
// as potentially compressed content.
func NewUsefulHandler(handler http.Handler, opts ...Options) http.Handler {
	return &Handler{
		handler: ch.CompressedHandler(handler),
		Log:     NewLog(opts...),
	}
}
