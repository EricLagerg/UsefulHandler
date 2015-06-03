package useful

import (
	"io"
	"net/http"
	"os"

	ch "github.com/EricLagerg/compressedhandler"
)

// Handler ia a wrapper for an http.Handler with an io.Writer for
// writing to log files.
type Handler struct {
	handler http.Handler
	out     io.Writer
}

// NewUsefulHandler returns a *Handler with logging capabilities as well
// as potentially compressed content.
func NewUsefulHandler(handler http.Handler) http.Handler {
	var out io.Writer

	switch LogDestination {
	case Stdout:
		out = os.Stdout
	case File:
		out = LogFile.file
	default:
		out = io.MultiWriter(os.Stdout, LogFile.file)
	}

	return &Handler{
		handler: ch.CompressedHandler(handler),
		out:     out,
	}
}
