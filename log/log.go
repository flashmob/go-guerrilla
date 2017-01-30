package log

import (
	"bufio"
	log "github.com/Sirupsen/logrus"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"
)

type Logger interface {
	log.FieldLogger
	WithConn(conn net.Conn) *log.Entry
	Reopen() error
	Change(newFile string) error
	GetLogDest() string
	SetLevel(level string)
	GetLevel() string
}

// Implements the Logger interface
type LoggerImpl struct {

	// satisfy the log.FieldLogger interface
	*log.Logger

	h LoggerHook
}

func NewLogger(dest string) Logger {
	logrus := log.New()
	logrus.Out = ioutil.Discard // we'll use the hook to output instead
	h, _ := NewLogrusHook(dest)
	logrus.Hooks.Add(h)
	l := &LoggerImpl{}
	l.Logger = logrus
	l.h = h
	return l
}

// SetLevel sets a log level, one of the LogLevels
func (l *LoggerImpl) SetLevel(level string) {
	var logLevel log.Level
	var err error
	if logLevel, err = log.ParseLevel(level); err != nil {
		return
	}
	l.Level = logLevel
	log.SetLevel(logLevel)
}

// GetLevel gets the current log level
func (l *LoggerImpl) GetLevel() string {
	return l.Level.String()
}

// Reopen closes the log file and re-opens it
func (l *LoggerImpl) Reopen() error {
	return l.h.Reopen()
}

// Change closes the old file, open a new one
func (l *LoggerImpl) Change(newFile string) error {
	return l.h.Change(newFile)
}

// Fgetname Gets the file name
func (l *LoggerImpl) GetLogDest() string {
	return l.h.GetLogDest()
}

// WithConn extends logrus to be able to log with a net.Conn
func (l *LoggerImpl) WithConn(conn net.Conn) *log.Entry {
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
	Change(newFile string) error
	GetLogDest() string
}
type LoggerHookImpl struct {
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
	defer hookMu.Unlock()
	hookMu.Lock()
	hook := LoggerHookImpl{fname: dest}
	err := hook.setup(dest)
	return &hook, err
}

// Setups sets the hook's writer w and file descriptor w
// assumes the hook.fd is closed and nil
func (hook *LoggerHookImpl) setup(dest string) error {

	if dest == "" || dest == "stderr" {
		hook.w = os.Stderr
	} else if dest == "stdout" {
		hook.w = os.Stdout
	} else if dest == "off" {
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
func (hook *LoggerHookImpl) openAppend(dest string) (err error) {
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
func (hook *LoggerHookImpl) openCreate(dest string) (err error) {
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
func (hook *LoggerHookImpl) Fire(entry *log.Entry) error {
	defer hookMu.Unlock()
	hookMu.Lock()
	if hook.fd != nil {
		// save the old hook
		oldhook := entry.Logger.Formatter
		defer func() {
			// set the back to the old hook after we're done
			entry.Logger.Formatter = oldhook
		}()
		// use the plain text hook
		entry.Logger.Formatter = hook.plainTxtFormatter
	}
	if line, err := entry.String(); err == nil {
		r := strings.NewReader(line)
		_, err = io.Copy(hook.w, r)
		if wb, ok := hook.w.(*bufio.Writer); ok {
			if err := wb.Flush(); err != nil {
				return err
			}
		}
		return err
	} else {
		return err
	}
}

// GetLogDest returns the destination of the log as a string
func (hook *LoggerHookImpl) GetLogDest() string {
	defer hookMu.Unlock()
	hookMu.Lock()
	return hook.fname
}

// Levels implements the logrus Hook interface
func (hook *LoggerHookImpl) Levels() []log.Level {
	return log.AllLevels
}

// close and re-open log file descriptor, which is a special feature of this hook
func (hook *LoggerHookImpl) Reopen() error {
	var err error
	defer hookMu.Unlock()
	hookMu.Lock()
	if hook.fd != nil {
		if err = hook.fd.Close(); err != nil {
			return err
		}
		if err := hook.openAppend(hook.fname); err != nil {
			return err
		}
	}
	return err

}

// Changes changes the destination to test
func (hook *LoggerHookImpl) Change(dest string) error {
	defer hookMu.Unlock()
	hookMu.Lock()
	if hook.fd != nil {
		// close the old destination
		hook.fd.Close()
		hook.fd = nil
		hook.w = nil
	}
	return hook.setup(dest)
}
