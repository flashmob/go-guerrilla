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
	Freopen()
	Frename(newFile string)
	Fgetname() string
	SetLevel(level string)
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
	h := newLogrusHook(dest)
	logrus.Hooks.Add(h)
	l := &LoggerImpl{}
	l.Logger = logrus
	l.h = h
	return l
}

var LogLevels = map[string]log.Level{
	"panic": log.PanicLevel,
	"error": log.ErrorLevel,
	"warn":  log.WarnLevel,
	"info":  log.InfoLevel,
	"debug": log.DebugLevel,
}

func (l *LoggerImpl) SetLevel(level string) {
	if v, ok := LogLevels[level]; ok {
		l.Level = v
	}
}

// Close the log file and re-open it
func (l *LoggerImpl) Freopen() {
	l.h.Freopen()
}

// Close old file, open a new one
func (l *LoggerImpl) Frename(newFile string) {
	l.h.Frename(newFile)
}

func (l *LoggerImpl) Fgetname() string {
	return l.h.Fgetname()
}

func (l *LoggerImpl) WithConn(conn net.Conn) *log.Entry {
	var addr string = "unknown"

	if conn != nil {
		addr = conn.RemoteAddr().String()
	}
	return l.WithField("addr", addr)
}

// custom logrus hook

// LoggerHook extends the log.Hook interface by adding Reopen() and Rename()
type LoggerHook interface {
	log.Hook
	Freopen()
	Frename(newFile string)
	Fgetname() string
}
type LoggerHookImpl struct {
	w io.Writer
	// ensure we do not lose entries while re-opening
	mu sync.Mutex
	// file descriptor, can be re-opened
	fd *os.File
	// filename to the file descriptor
	fname string

	// txtFormatter that doesn't use colors
	plainTxtFormatter *log.TextFormatter
}

// newLogrusHook creates a new hook. dest can be a file name or one of the following strings:
// "stderr" - log to stderr, lines will be written to os.Stdout
// "stdout" - log to stdout, lines will be written to os.Stdout
// "off" - no log, lines will be written to ioutil.Discard
func newLogrusHook(dest string) LoggerHook {
	var w io.Writer
	hook := LoggerHookImpl{fname: dest}
	if dest == "" || dest == "stderr" {
		w = os.Stderr
	} else if dest == "stdout" {
		w = os.Stdout
	} else if dest == "off" {
		w = ioutil.Discard
	} else {
		if _, err := os.Stat(dest); err == nil {
			// fire exists open the file for appending
			if fd, err := os.OpenFile(dest, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
				w = bufio.NewWriter(fd)
				hook.fd = fd
			} else {
				log.WithError(err).Error("Could not open log file for appending")
				w = os.Stderr
			}
		} else {
			// create the file
			if fd, err := os.Create(dest); err == nil {
				w = bufio.NewWriter(fd)
				hook.fd = fd
			} else {
				log.WithError(err).Error("Could not create log file")
				w = os.Stderr
			}
		}
	}
	if hook.fd != nil {
		hook.plainTxtFormatter = &log.TextFormatter{DisableColors: true}
	}
	hook.w = w
	return &hook
}

// Fire implements the logrus Hook interface. It disables color text formatting if writing to a file
func (hook *LoggerHookImpl) Fire(entry *log.Entry) error {
	defer hook.mu.Unlock()
	hook.mu.Lock()
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
			wb.Flush()
		}
		return err
	} else {
		return err
	}

}

func (hook *LoggerHookImpl) Fgetname() string {
	return hook.fname
}

// Levels implements the logrus Hook interface
func (hook *LoggerHookImpl) Levels() []log.Level {
	return log.AllLevels
}

// close and re-open log files, which is a special feature of this hook
func (hook *LoggerHookImpl) Freopen() {
	defer hook.mu.Unlock()
	hook.mu.Lock()
}

// Rename the log file
func (hook *LoggerHookImpl) Frename(newFile string) {
	defer hook.mu.Unlock()
	hook.mu.Lock()
}
