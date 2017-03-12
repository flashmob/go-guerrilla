package log

import (
	"bufio"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"

	"github.com/flashmob/go-guerrilla/dashboard"
)

type Logger interface {
	log.FieldLogger
	WithConn(conn net.Conn) *log.Entry
	Reopen() error
	GetLogDest() string
	SetLevel(level string)
	GetLevel() string
	IsDebug() bool
	AddHook(h log.Hook)
}

// Implements the Logger interface
// It's a logrus logger wrapper that contains an instance of our LoggerHook
type HookedLogger struct {

	// satisfy the log.FieldLogger interface
	*log.Logger

	h LoggerHook
}

type loggerCache map[string]Logger

// loggers store the cached loggers created by NewLogger
var loggers struct {
	cache loggerCache
	// mutex guards the cache
	sync.Mutex
}

// GetLogger returns a struct that implements Logger (i.e HookedLogger) with a custom hook.
// It may be new or already created, (ie. singleton factory pattern)
// The hook has been initialized with dest
// dest can can be a path to a file, or the following string values:
// "off" - disable any log output
// "stdout" - write to standard output
// "stderr" - write to standard error
// If the file doesn't exists, a new file will be created. Otherwise it will be appended
// Each Logger returned is cached on dest, subsequent call will get the cached logger if dest matches
// If there was an error, the log will revert to stderr instead of using a custom hook

func GetLogger(dest string) (Logger, error) {
	loggers.Lock()
	defer loggers.Unlock()
	if loggers.cache == nil {
		loggers.cache = make(loggerCache, 1)
	} else {
		if l, ok := loggers.cache[dest]; ok {
			// return the one we found in the cache
			return l, nil
		}
	}
	logrus := log.New()
	// we'll use the hook to output instead
	logrus.Out = ioutil.Discard

	l := &HookedLogger{}
	l.Logger = logrus

	// cache it
	loggers.cache[dest] = l

	// setup the hook
	if h, err := NewLogrusHook(dest); err != nil {
		// revert back to stderr
		logrus.Out = os.Stderr
		return l, err
	} else {
		logrus.Hooks.Add(h)
		l.h = h
	}

	// add the dashboard hook
	logrus.Hooks.Add(dashboard.LogHook)

	return l, nil

}

// AddHook adds a new logrus hook
func (l *HookedLogger) AddHook(h log.Hook) {
	log.AddHook(h)
}

func (l *HookedLogger) IsDebug() bool {
	return l.GetLevel() == log.DebugLevel.String()
}

// SetLevel sets a log level, one of the LogLevels
func (l *HookedLogger) SetLevel(level string) {
	var logLevel log.Level
	var err error
	if logLevel, err = log.ParseLevel(level); err != nil {
		return
	}
	l.Level = logLevel
	log.SetLevel(logLevel)
}

// GetLevel gets the current log level
func (l *HookedLogger) GetLevel() string {
	return l.Level.String()
}

// Reopen closes the log file and re-opens it
func (l *HookedLogger) Reopen() error {
	return l.h.Reopen()
}

// Fgetname Gets the file name
func (l *HookedLogger) GetLogDest() string {
	return l.h.GetLogDest()
}

// WithConn extends logrus to be able to log with a net.Conn
func (l *HookedLogger) WithConn(conn net.Conn) *log.Entry {
	var addr string = "unknown"

	if conn != nil {
		addr = conn.RemoteAddr().String()
	}
	return l.WithField("addr", addr)
}

// custom logrus hook

// hookMu ensures all io operations are synced. Always on exported functions
var hookMu sync.Mutex

// LoggerHook extends the log.Hook interface by adding Reopen() and Rename()
type LoggerHook interface {
	log.Hook
	Reopen() error
	GetLogDest() string
}
type LogrusHook struct {
	w io.Writer
	// file descriptor, can be re-opened
	fd *os.File
	// filename to the file descriptor
	fname string
	// txtFormatter that doesn't use colors
	plainTxtFormatter *log.TextFormatter

	mu sync.Mutex
}

// newLogrusHook creates a new hook. dest can be a file name or one of the following strings:
// "stderr" - log to stderr, lines will be written to os.Stdout
// "stdout" - log to stdout, lines will be written to os.Stdout
// "off" - no log, lines will be written to ioutil.Discard
func NewLogrusHook(dest string) (LoggerHook, error) {
	hookMu.Lock()
	defer hookMu.Unlock()
	hook := LogrusHook{fname: dest}
	err := hook.setup(dest)
	return &hook, err
}

type OutputOption int

const (
	OutputStderr OutputOption = 1 + iota
	OutputStdout
	OutputOff
	OutputNull
	OutputFile
)

var outputOptions = [...]string{
	"stderr",
	"stdout",
	"off",
	"",
	"file",
}

func (o OutputOption) String() string {
	return outputOptions[o-1]
}

func parseOutputOption(str string) OutputOption {
	switch str {
	case "stderr":
		return OutputStderr
	case "stdout":
		return OutputStdout
	case "off":
		return OutputOff
	case "":
		return OutputNull
	}
	return OutputFile
}

// Setup sets the hook's writer w and file descriptor fd
// assumes the hook.fd is closed and nil
func (hook *LogrusHook) setup(dest string) error {

	out := parseOutputOption(dest)
	if out == OutputNull || out == OutputStderr {
		hook.w = os.Stderr
	} else if out == OutputStdout {
		hook.w = os.Stdout
	} else if out == OutputOff {
		hook.w = ioutil.Discard
	} else {
		if _, err := os.Stat(dest); err == nil {
			// file exists open the file for appending
			if err := hook.openAppend(dest); err != nil {
				return err
			}
		} else {
			// create the file
			if err := hook.openCreate(dest); err != nil {
				return err
			}
		}
	}
	// disable colors when writing to file
	if hook.fd != nil {
		hook.plainTxtFormatter = &log.TextFormatter{DisableColors: true}
	}
	return nil
}

// openAppend opens the dest file for appending. Default to os.Stderr if it can't open dest
func (hook *LogrusHook) openAppend(dest string) (err error) {
	fd, err := os.OpenFile(dest, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.WithError(err).Error("Could not open log file for appending")
		hook.w = os.Stderr
		hook.fd = nil
		return
	}
	hook.w = bufio.NewWriter(fd)
	hook.fd = fd
	return
}

// openCreate creates a new dest file for appending. Default to os.Stderr if it can't open dest
func (hook *LogrusHook) openCreate(dest string) (err error) {
	fd, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.WithError(err).Error("Could not create log file")
		hook.w = os.Stderr
		hook.fd = nil
		return
	}
	hook.w = bufio.NewWriter(fd)
	hook.fd = fd
	return
}

// Fire implements the logrus Hook interface. It disables color text formatting if writing to a file
func (hook *LogrusHook) Fire(entry *log.Entry) error {
	hookMu.Lock()
	defer hookMu.Unlock()
	if hook.fd != nil {
		// save the old hook
		oldhook := entry.Logger.Formatter
		defer func() {
			// set the back to the old hook after we're done
			entry.Logger.Formatter = oldhook
		}()
		// use the plain text hook
		entry.Logger.Formatter = hook.plainTxtFormatter
		// todo : `go go test -v -race` detected a race condition, try log.SetFormatter()
	}
	if line, err := entry.String(); err == nil {
		r := strings.NewReader(line)
		if _, err = io.Copy(hook.w, r); err != nil {
			return err
		}
		if wb, ok := hook.w.(*bufio.Writer); ok {
			if err := wb.Flush(); err != nil {
				return err
			}
			if hook.fd != nil {
				hook.fd.Sync()
			}
		}
		return err
	} else {
		return err
	}
}

// GetLogDest returns the destination of the log as a string
func (hook *LogrusHook) GetLogDest() string {
	hookMu.Lock()
	defer hookMu.Unlock()
	return hook.fname
}

// Levels implements the logrus Hook interface
func (hook *LogrusHook) Levels() []log.Level {
	return log.AllLevels
}

// Reopen closes and re-open log file descriptor, which is a special feature of this hook
func (hook *LogrusHook) Reopen() error {
	hookMu.Lock()
	defer hookMu.Unlock()
	var err error
	if hook.fd != nil {
		if err = hook.fd.Close(); err != nil {
			return err
		}
		// The file could have been re-named by an external program such as logrotate(8)
		if _, err := os.Stat(hook.fname); err != nil {
			// The file doesn't exist,create a new one.
			return hook.openCreate(hook.fname)
		} else {
			return hook.openAppend(hook.fname)
		}
	}
	return err

}
