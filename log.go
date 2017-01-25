package guerrilla

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
}

// Implements the Logger interface
type LoggerImpl struct {

	// satisfy the log.FieldLogger interface
	*log.Logger

	h LoggerHook
}

func NewLogger(dest string) Logger {
	logrus := log.New()
	h := newLogrusHook(dest)
	logrus.Hooks.Add(h)
	l := &LoggerImpl{}
	l.Logger = logrus
	l.h = h
	return l
}

// Close the log file and re-open it
func (l *LoggerImpl) Reopen() {
	l.h.Reopen()
}

// Close old file, open a new one
func (l *LoggerImpl) Rename(newFile string) {
	l.h.Rename(newFile)
}

// If no log_file set, use the mainlog
func newServerLogger(sc *ServerConfig) {

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
	Reopen()
	Rename(newFile string)
}

type LoggerHookImpl struct {
	w io.Writer

	// ensure we do not lose entries while re-opening
	mu sync.Mutex
	// file descriptor, can be re-opened
	fd *os.File
	// filename to the file descriptor
	fname string
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
	hook.w = w
	return &hook
}

// Fire implements the logrus Hook interface
func (hook *LoggerHookImpl) Fire(entry *log.Entry) error {
	defer hook.mu.Unlock()
	hook.mu.Lock()
	if line, err := entry.String(); err == nil {
		r := strings.NewReader(line)
		_, err = io.Copy(hook.w, r)
		return err
	} else {
		return err
	}

}

// Levels implements the logrus Hook interface
func (hook *LoggerHookImpl) Levels() []log.Level {
	return log.AllLevels
}

// close and re-open log files, which is a special feature of this hook
func (hook *LoggerHookImpl) Reopen() {
	defer hook.mu.Unlock()
	hook.mu.Lock()
}

// Rename the log file
func (hook *LoggerHookImpl) Rename(newFile string) {
	defer hook.mu.Unlock()
	hook.mu.Lock()
}
