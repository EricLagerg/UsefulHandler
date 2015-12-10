package useful

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ErrUnHijackable indicates an unhijackable connection. I.e., (one of)
// the underlying http.ResponseWriter(s) doesn't support the http.Hijacker
// interface.
var ErrUnHijackable = errors.New("A(n) underlying ResponseWriter doesn't support the http.Hijacker interface")

// These format strings correspond with the log formats described in
// https://httpd.apache.org/docs/2.2/mod/mod_log_config.html
var (
	// CommonLog is "%h %l %u %t \"%r\" %>s %b"
	CommonLog commonLog = "%s - - [%s] \"%s\" %d %d\n"

	// CommonLogWithVHost is "%v %h %l %u %t \"%r\" %>s %b"
	CommonLogWithVHost commonLogWithVHost = "- %s - - [%s] \"%s\" %d %d\n"

	// NCSALog is
	// "%h %l %u %t \"%r\" %>s %b \"%{Referer}i\" \"%{User-agent}i\""
	NCSALog ncsaLog = "%s - - [%s] \"%s\" %d %d \"%s\" \"%s\"\n"

	// RefererLog is "%{Referer}i -> %U"
	RefererLog refererLog = "%s -> %s\n"

	// AgentLog is "%{User-agent}i"
	AgentLog agentLog = "%s\n"
)

type (
	commonLog          string
	commonLogWithVHost string
	ncsaLog            string
	refererLog         string
	agentLog           string
)

// ApacheLogRecord is a structure containing the necessary information
// to write a proper log in the ApacheFormatPattern.
type ApacheLogRecord struct {
	http.ResponseWriter
	Logger

	ip            string
	time          time.Time
	method        string
	uri           string
	protocol      string
	status        int
	responseBytes int64
	elapsedTime   time.Duration
	referer       string
	agent         string
}

// Hijack implements the http.Hijacker interface to allow connection
// hijacking.
func (a *ApacheLogRecord) Hijack() (rwc net.Conn, buf *bufio.ReadWriter, err error) {
	hj, ok := a.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, ErrUnHijackable
	}
	return hj.Hijack()
}

// Log will log an entry to its io.Writer.
func (l *Log) Log(r ApacheLogRecord) {
	l.Lock()
	n, err := r.WriteTo(l.out)
	if err != nil {
		return
	}
	if l.size+int64(n) >= l.MaxFileSize {
		l.Rotate()
	}
	l.size += int64(n)
	l.Unlock()
}

func (r ApacheLogRecord) WriteTo(w io.Writer) (n int64, err error) {
	nn, err := r.Logger.WriteLog(w, r)
	return int64(nn), err
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

	record := ApacheLogRecord{
		ResponseWriter: rw,
		Logger:         h.Log,
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
	h.handler.ServeHTTP(&record, r)
	finishTime := time.Now()

	record.time = finishTime.UTC()
	record.elapsedTime = finishTime.Sub(startTime)

	h.Log.Log(record)
}
