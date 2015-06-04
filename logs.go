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
	B        = Byte
	Kilobyte = 1024 * Byte
	KB       = Kilobyte
	Megabyte = 1024 * Kilobyte
	MB       = Megabyte
	Gigabyte = 1024 * Megabyte
	GB       = Gigabyte
	Terabyte = 1024 * Gigabyte
	TB       = Terabyte
)

type dest uint8

// Locations for log writing.
const (
	Stdout dest = iota
	File
	Both
)

// archPrefix is the temporary archive file's prefix before
// randName appends a random string of digits to the end.
const archPrefix = "._archive"

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
)

// Log is a structure with our open file we log to, the size of said file
// (measured by the number of bytes written to it, or it's size on
// initialization), our current writer (usually Stdout and the
// aforementioned file), our pool of random names, and a mutex lock
// to keep race conditions from tripping us up.
type Log struct {
	file          *os.File  // pointer to the open file
	size          int64     // number of bytes written to file
	out           io.Writer // current io.Writer
	pool          *randPool // pool of random names
	*sync.RWMutex           // mutex for locking
}

// SetLog sets LogFile and starts the check for 'cur'.
func SetLog() {
	var err error

	LogFile, err = NewLog()
	if err != nil {
		panic(err)
	}

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

	log := &Log{
		file,
		size,
		nil,
		newRandPool(25),
		&sync.RWMutex{},
	}

	log.SetWriter(true)

	return log, nil
}

// newFile returns a 'new' file to write logs to.
// It's simply a wrapper around os.OpenFile.
// While it says 'new', it'll return an already existing log file
// if one exists.
func newFile() (file *os.File, err error) {
	file, err = os.OpenFile(LogName,
		os.O_RDWR|os.O_APPEND|os.O_CREATE, 0600)
	return
}

// Start begins the check for 'cur'.
// TOOD: Implement this better.
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

	u := strings.LastIndex(highest[:], "_")
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

	// For speed.
	randName := l.pool.get()

	// Rename so we can release our lock on the file asap.
	os.Rename(LogName, randName)

	// Replace our physical file.
	l.file, err = newFile()
	if err != nil {
		panic(err)
	}

	// Reset the size.
	l.size = 0

	// Reset the writer (underlying io.Writer would otherwise point to the
	// fd of the old, renamed file).
	l.SetWriter(false)

	// We don't need to do any other actions that could cause race
	// conditions, so unlock the file.
	l.Unlock()

	// Place the used name back into the pool for future use.
	l.pool.put(randName)

	// From here on out we don't need to worry about time because we've
	// already moved the Log file and created a new, unlocked one for
	// our handler to write to.
	path := filepath.Join(ArchiveDir, LogName)

	// E.g., "access.log_01.gz"
	// We throw in the underscore before the number to try to help
	// identify our numbering scheme even if the user picks a wacky
	// file that includes numbers and stuff.
	archiveName := fmt.Sprintf("%s#%010d_.gz", path, cur)
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

// SetWriter sets Log's writer depending on LogDestination.
func (l *Log) SetWriter(init bool) {
	// Catch initialization case without breaking up any more of the
	// logic.
	if init {
		l.out = io.MultiWriter(os.Stdout, l.file)
		return
	}

	switch LogDestination {
	case Stdout:
		l.out = os.Stdout
	case File:
		l.out = LogFile.file
	default:
		l.out = io.MultiWriter(os.Stdout, LogFile.file)
	}
}

// randPool is a pool of random names used for rotating log files.
type randPool struct {
	c chan string
	*sync.Mutex
}

// newRandPool creates a new pool of random names and immediately
// initializes the pool with N new names.
func newRandPool(n int) *randPool {
	pool := &randPool{
		make(chan string, n),
		&sync.Mutex{},
	}

	for i := 0; i < n; i++ {
		pool.put(randName(archPrefix))
	}

	return pool
}

// get gets a name from the pool, or generates a new name if none
// exist.
func (p *randPool) get() (s string) {
	p.Lock()
	defer p.Unlock()

	select {
	case s = <-p.c:
		// get a name from the pool
	default:
		return randName(archPrefix)
	}
	return
}

// put puts a new name (back) into the pool, or discards it if the pool
// is full.
func (p *randPool) put(s string) {
	p.Lock()
	defer p.Unlock()

	select {
	case p.c <- s:
		// place back into pool
	default:
		// discard if pool is full
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
		_, err := os.Stat(name)
		if os.IsExist(err) {
			if nconflict++; nconflict > 10 {
				rand = reseed()
			}
			continue
		}
		break
	}
	return
}
