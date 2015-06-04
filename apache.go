package useful

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// These format strings correspond with the log formats described in
// https://httpd.apache.org/docs/2.2/mod/mod_log_config.html
const (
	// CommonLog is "%h %l %u %t \"%r\" %>s %b"
	CommonLog = "%s - - [%s] \"%s %d %d\" %f\n"

	// CommonLogWithVHost is "%v %h %l %u %t \"%r\" %>s %b"
	CommonLogWithVHost = "%s %s - - [%s] \"%s %d %d\" %f\n"

	// NCSALog is
	// "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\""
	NCSALog = "%s - - [%s] \"%s %d %d\" %f\n \"%s\" \"%s\""

	// RefererLog is "%{Referer}i -> %U"
	RefererLog = "%s -> %s"

	// AgentLog is "%{User-agent}i"
	AgentLog = "%s"
)

// ApacheLogRecord is a structure containing the necessary information
// to write a proper log in the ApacheFormatPattern.
type ApacheLogRecord struct {
	http.ResponseWriter

	ip                    string
	time                  time.Time
	method, uri, protocol string
	status                int
	responseBytes         int64
	elapsedTime           time.Duration
	referer, agent        string
}

// Log will log an entry to the io.Writer specified by LogDestination.
func (r *ApacheLogRecord) Log(out io.Writer) {
	LogFile.Lock()
	defer LogFile.Unlock()

	timeFormatted := r.time.Format("02/Jan/2006 03:04:05")
	requestLine := fmt.Sprintf("%s %s %s", r.method, r.uri, r.protocol)

	var n int

	switch LogFormat {
	case CommonLog:
		n, _ = fmt.Fprintf(LogFile.out, CommonLog, r.ip, timeFormatted,
			requestLine, r.status, r.responseBytes, r.elapsedTime.Seconds())
	case CommonLogWithVHost:
		n, _ = fmt.Fprintf(LogFile.out, CommonLogWithVHost, "-", r.ip,
			timeFormatted, requestLine, r.status, r.responseBytes,
			r.elapsedTime.Seconds())
	case NCSALog:
		n, _ = fmt.Fprintf(LogFile.out, NCSALog, r.ip,
			timeFormatted, requestLine, r.status, r.responseBytes,
			r.elapsedTime.Seconds(), r.referer, r.agent)
	case RefererLog:
		n, _ = fmt.Fprintf(LogFile.out, RefererLog, r.referer, r.uri)
	case AgentLog:
		n, _ = fmt.Fprintf(LogFile.out, AgentLog, r.agent)
	default:
		// Common log.
		n, _ = fmt.Fprintf(LogFile.out, CommonLog, r.ip, timeFormatted,
			requestLine, r.status, r.responseBytes, r.elapsedTime.Seconds())
	}

	if LogFile.size+int64(n) >= MaxFileSize {
		go LogFile.Rotate()
	}

	LogFile.size += int64(n)
}

// Write fulfills the Write method of the http.ResponseWriter interface.
func (r *ApacheLogRecord) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.responseBytes += int64(n)
	return n, err
}

// WriteHeader fulfills the WriteHeader method of the http.ResponseWriter
// interface.
func (r *ApacheLogRecord) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// ServeHTTP fulfills the ServeHTTP method of the http.Handler interface.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if colon := strings.LastIndex(clientIP, ":"); colon != -1 {
		clientIP = clientIP[:colon]
	}

	record := &ApacheLogRecord{
		ResponseWriter: rw,
		ip:             clientIP,
		time:           time.Time{},
		method:         r.Method,
		uri:            r.RequestURI,
		protocol:       r.Proto,
		status:         http.StatusOK,
		elapsedTime:    time.Duration(0),
		referer:        r.Referer(),
		agent:          r.UserAgent(),
	}

	startTime := time.Now()
	h.handler.ServeHTTP(record, r)
	finishTime := time.Now()

	record.time = finishTime.UTC()
	record.elapsedTime = finishTime.Sub(startTime)

	record.Log(LogFile.out)
}
