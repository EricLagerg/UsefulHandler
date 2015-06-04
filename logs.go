package useful

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// For ease of use.
const (
	Byte     = 1
	Kilobyte = 1024 * Byte
	Megabyte = 1024 * Kilobyte
	Gigabyte = 1024 * Megabyte
	Terabyte = 1024 * Gigabyte
)

type dest uint8

// Locations for log writing.
const (
	Stdout dest = iota
	File
	Both
)

var (
	// LogFormat determines the format of the log. Most standard
	// formats found in Apache's mod_log_config docs are supported.
	LogFormat = CommonLog

	// LogDestination determines where the Handler will write to.
	// By default it writes to Stdout and LogName.
	LogDestination = Both

	// LogName is the name of the log the handler will write to.
	// It defaults to "access.log", but can be set to anything you
	// want.
	LogName = "access.log"

	// ArchiveDir is the directory where the archives will be stored.
	// If set to "" (empty string) it'll be set to the current directory.
	// It defaults to "archives", so it'll look a little something like
	// this: '/home/user/files/archives/'
	ArchiveDir = "archives"

	// MaxFileSize is the maximum size of a log file in bytes.
	// It defaults to 1 Gigabyte (multiple of 1024, not 1000),
	// but can be set to anything you want.
	//
	// Log files larger than this size will be compressed into
	// archive files.
	MaxFileSize int64 = 1 * Gigabyte

	// LogFile is the active Log.
	LogFile *Log

	// cur is the current log iteration. E.g., if there are 10
	// archived logs, cur will be 11.
	cur int64

	// out is the current io.Writer
	out io.Writer
)

// Log is a wrapper for a log file to provide mutex locks.
type Log struct {
	file          *os.File // pointer to the open file
	size          int64    // number of bytes written to file
	*sync.RWMutex          // mutex for locking
}

func SetLog() {
	LogFile, _ = NewLog()
	LogFile.Start()
}

// NewLog returns a new Log initialized to the default values.
// If no log file exists with the name specified in 'LogName'
// it'll create a new one, otherwise it opens 'LogName'.
// If it cannot create or open a file it'll return nil for *Log
// and the applicable error.
func NewLog() (*Log, error) {
	file, err := newFile()
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		panic(err)
	}
	size := stat.Size()

	return &Log{file, size, &sync.RWMutex{}}, nil
}

func newFile() (*os.File, error) {
	file, err := os.OpenFile(LogName,
		os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)

	if err != nil {
		return nil, err
	}

	return file, nil
}

// Start beings the logging.
func (l *Log) Start() {

	// Check for the current archive log number. It *should* be fine
	// inside a Goroutine because, unless there's a *ton* of archive
	// files and the current Log is just shy of MaxFileSize, it'll
	// finish before Log fills up and needs to be rotated.
	go findCur()
}

// findCur finds the current archive log number. If any errors occur it'll
func findCur() {
	dir, err := os.Open(ArchiveDir)
	if err != nil {
		panic(err)
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)

	// 0 names means the directory is empty, so cur *has* to be 0.
	if len(names) == 0 {
		cur = 0
		return
	}

	// Sort the strings. Our naming scheme, "#%02d_" will allow us to
	// select the last string in the slice once it's ordered
	// in increasing order.
	sort.Strings(names)

	highest := names[len(names)-1]

	// Okay, we've found some gzipped files.
	if !strings.HasSuffix(highest, "_.gz") {
		cur = 0
		return
	}

	h := strings.LastIndex(highest, "#")
	if h == -1 {
		panic("Could not find current file number.")
	}

	u := strings.LastIndex(highest[h:], "_")
	if u == -1 {
		panic("Could not find current file number.")
	}

	cur, err = strconv.ParseInt(highest[h+1:u-1], 10, 64)
	if err != nil {
		panic(err)
	}
}

// Rotate will rotate the logs so that the current (theoretically
// full) log will be compressed and added to the archive and a new
// log generated.
func (l *Log) Rotate() {
	var err error

	l.Lock()

	randName := randName("ARCHIVE")

	// Rename so we can release our lock on the file asap.
	os.Rename(LogName, randName)

	// Reset our Log.
	l.file, err = newFile()
	if err != nil {
		panic(err)
	}

	l.size = 0
	setWriter()
	l.Unlock()

	// From here on out we don't need to worry about time because we've
	// already moved the Log file and created a new, unlocked one for
	// our handler to write to.
	path := filepath.Join(ArchiveDir, LogName)

	// E.g., "access.log_01.gz"
	// We throw in the underscore before the number to try to help
	// identify our numbering scheme even if the user picks a wacky
	// file that includes numbers and stuff.
	archiveName := fmt.Sprintf("%s#%02d_.gz", path, cur)
	cur++

	archive, err := os.Create(archiveName)
	if err != nil {
		panic(err)
	}
	defer archive.Close()

	oldLog, err := os.Open(randName)
	if err != nil {
		panic(err)
	}
	defer oldLog.Close()

	gzw, err := gzip.NewWriterLevel(archive, gzip.BestCompression)
	if err != nil {
		panic(err)
	}
	defer gzw.Close()

	_, err = io.Copy(gzw, oldLog)
	if err != nil {
		panic(err)
	}

	err = os.Remove(randName)
	if err != nil {
		panic(err)
	}
}

func setWriter() {
	switch LogDestination {
	case Stdout:
		out = os.Stdout
	case File:
		out = LogFile.file
	default:
		out = io.MultiWriter(os.Stdout, LogFile.file)
	}
}

// Borrowed from https://golang.org/src/io/ioutil/tempfile.go#L19

var rand uint32
var randmu sync.Mutex

func reseed() uint32 {
	return uint32(time.Now().UnixNano() + int64(os.Getpid()))
}

func nextSuffix() string {
	randmu.Lock()
	r := rand
	if r == 0 {
		r = reseed()
	}
	r = r*1664525 + 1013904223 // constants from Numerical Recipes
	rand = r
	randmu.Unlock()
	return strconv.Itoa(int(1e9 + r%1e9))[1:]
}

func randName(prefix string) (name string) {
	nconflict := 0
	for i := 0; i < 10000; i++ {
		name = prefix + nextSuffix()
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			if nconflict++; nconflict > 10 {
				rand = reseed()
			}
			continue
		}
		defer f.Close()
		break
	}
	return
}
