package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"runtime/pprof"

	useful "github.com/EricLagerg/UsefulHandler"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "Hello, world!")
	})

	opts := useful.Options{
		Logger:      useful.NCSALog,
		Destination: useful.Both,
		ArchiveDir:  "archives",
		LogName:     "access.log",
		MaxFileSize: 2 * useful.Megabyte,
	}
	myHandler := useful.NewUsefulHandler(handler, opts)

	http.Handle("/", myHandler)
	server := http.Server{
		Addr:    ":1234",
		Handler: nil,
	}
	server.ListenAndServe()
}
