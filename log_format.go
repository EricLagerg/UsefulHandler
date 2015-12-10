package useful

import (
	"fmt"
	"io"
	"strings"
)

// Logger is the interface implemented by log types to print an
// ApacheLogRecord in the desired format.
type Logger interface {
	WriteLog(w io.Writer, r ApacheLogRecord) (n int, err error)
}

const timeFormat = "02/Jan/2006 03:04:05"

// timeRequest returns the formatted time of the request and the request line.
func (r ApacheLogRecord) formattedTimeRequest() (string, string) {
	return r.time.Format(timeFormat), strings.Join([]string{r.method, r.uri, r.protocol}, " ")
}

func (l commonLog) WriteLog(w io.Writer, r ApacheLogRecord) (n int, err error) {
	timeFormatted, requestLine := r.formattedTimeRequest()
	return fmt.Fprintf(w, string(l), r.ip, timeFormatted, requestLine, r.status, r.responseBytes)
}

func (l commonLogWithVHost) WriteLog(w io.Writer, r ApacheLogRecord) (n int, err error) {
	timeFormatted, requestLine := r.formattedTimeRequest()
	return fmt.Fprintf(w, string(l), r.ip, timeFormatted, requestLine, r.status, r.responseBytes)
}

func (l ncsaLog) WriteLog(w io.Writer, r ApacheLogRecord) (n int, err error) {
	timeFormatted, requestLine := r.formattedTimeRequest()
	return fmt.Fprintf(w, string(l), r.ip, timeFormatted, requestLine, r.status, r.responseBytes, r.referer, r.agent)
}

func (l refererLog) WriteLog(w io.Writer, r ApacheLogRecord) (n int, err error) {
	return fmt.Fprintf(w, string(l), r.referer, r.uri)
}

func (l agentLog) WriteLog(w io.Writer, r ApacheLogRecord) (n int, err error) {
	return fmt.Fprintf(w, string(l), r.agent)
}
