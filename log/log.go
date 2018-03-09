package log

import (
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"net"
	"os"
	"sync"
)

// The following are taken from logrus
const (
	// PanicLevel level, highest level of severity. Logs and then calls panic with the
	// message passed to Debug, Info, ...
	PanicLevel Level = iota
	// FatalLevel level. Logs and then calls `os.Exit(1)`. It will exit even if the
	// logging level is set to Panic.
	FatalLevel
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	// Commonly used for hooks to send errors to an error tracking service.
	ErrorLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
)

type Level uint8

// Convert the Level to a string. E.g. PanicLevel becomes "panic".
func (level Level) String() string {
	switch level {
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warning"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	case PanicLevel:
		return "panic"
	}

	return "unknown"
}

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

	// destination, file name or "stderr", "stdout" or "off"
	dest string

	oo OutputOption
}

type loggerKey struct {
	dest, level string
}

type loggerCache map[loggerKey]Logger

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

func GetLogger(dest string, level string) (Logger, error) {
	loggers.Lock()
	defer loggers.Unlock()
	key := loggerKey{dest, level}
	if loggers.cache == nil {
		loggers.cache = make(loggerCache, 1)
	} else {
		if l, ok := loggers.cache[key]; ok {
			// return the one we found in the cache
			return l, nil
		}
	}
	o := parseOutputOption(dest)
	logrus, err := newLogrus(o, level)
	if err != nil {
		return nil, err
	}
	l := &HookedLogger{dest: dest}
	l.Logger = logrus

	// cache it
	loggers.cache[key] = l

	if o != OutputFile {
		return l, nil
	}
	// we'll use the hook to output instead
	logrus.Out = ioutil.Discard
	// setup the hook
	if h, err := NewLogrusHook(dest); err != nil {
		// revert back to stderr
		logrus.Out = os.Stderr
		return l, err
	} else {
		logrus.Hooks.Add(h)
		l.h = h
	}

	return l, nil

}

func newLogrus(o OutputOption, level string) (*log.Logger, error) {
	logLevel, err := log.ParseLevel(level)
	if err != nil {
		return nil, err
	}
	var out io.Writer

	if o != OutputFile {
		if o == OutputNull || o == OutputStderr {
			out = os.Stderr
		} else if o == OutputStdout {
			out = os.Stdout
		} else if o == OutputOff {
			out = ioutil.Discard
		}
	} else {
		// we'll use a hook to output instead
		out = ioutil.Discard
	}

	logger := &log.Logger{
		Out:       out,
		Formatter: new(log.TextFormatter),
		Hooks:     make(log.LevelHooks),
		Level:     logLevel,
	}

	return logger, nil
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
	log.SetLevel(logLevel)
}

// GetLevel gets the current log level
func (l *HookedLogger) GetLevel() string {
	return l.Level.String()
}

// Reopen closes the log file and re-opens it
func (l *HookedLogger) Reopen() error {
	if l.h == nil {
		return nil
	}
	return l.h.Reopen()
}

// GetLogDest Gets the file name
func (l *HookedLogger) GetLogDest() string {
	return l.dest
}

// WithConn extends logrus to be able to log with a net.Conn
func (l *HookedLogger) WithConn(conn net.Conn) *log.Entry {
	var addr string = "unknown"

	if conn != nil {
		addr = conn.RemoteAddr().String()
	}
	return l.WithField("addr", addr)
}
