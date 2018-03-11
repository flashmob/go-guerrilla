package log

import (
	"bufio"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
)

// custom logrus hook

// hookMu ensures all io operations are synced. Always on exported functions
var hookMu sync.Mutex

// LoggerHook extends the log.Hook interface by adding Reopen() and Rename()
type LoggerHook interface {
	log.Hook
	Reopen() error
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
			// The file doesn't exist, create a new one.
			return hook.openCreate(hook.fname)
		} else {
			return hook.openAppend(hook.fname)
		}
	}
	return err

}
