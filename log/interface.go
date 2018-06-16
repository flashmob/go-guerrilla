package log

import (
	"github.com/sirupsen/logrus"
	"net"
)

type GenericHook interface {
	Levels() []Level
	Fire(GenericEntry) error
	Reopen() error
}

type Fields map[string]interface{}

type GenericEntry interface {
	String() (string, error)
	WithError(err error) GenericEntry
	WithField(key string, value interface{}) GenericEntry
	WithFields(fields Fields) GenericEntry
	Debug(args ...interface{})
	Print(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Warning(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Panic(args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Printf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Panicf(format string, args ...interface{})
	Debugln(args ...interface{})
	Infoln(args ...interface{})
	Println(args ...interface{})
	Warnln(args ...interface{})
	Warningln(args ...interface{})
	Errorln(args ...interface{})
	Fatalln(args ...interface{})
	Panicln(args ...interface{})
}

type LogrusEntryAdapter struct {
	e *logrus.Entry
}

func (adapter *LogrusEntryAdapter) String() (string, error) {
	return adapter.e.String()
}

func (adapter *LogrusEntryAdapter) WithError(err error) GenericEntry {
	entry := new(LogrusEntryAdapter)
	entry.e = adapter.e.WithError(err)
	return entry
}

func (adapter *LogrusEntryAdapter) WithField(key string, value interface{}) GenericEntry {
	entry := new(LogrusEntryAdapter)
	entry.e = adapter.e.WithField(key, value)
	return entry
}

func (adapter *LogrusEntryAdapter) WithFields(fields Fields) GenericEntry {
	f := make(logrus.Fields)
	for k, v := range fields {
		f[k] = v
	}
	entry := new(LogrusEntryAdapter)
	entry.e = adapter.e.WithFields(f)
	return entry
}

func (adapter *LogrusEntryAdapter) Debug(args ...interface{}) {
	adapter.e.Debug(args...)
}

func (adapter *LogrusEntryAdapter) Print(args ...interface{}) {
	adapter.e.Print(args...)
}

func (adapter *LogrusEntryAdapter) Info(args ...interface{}) {
	adapter.e.Info(args...)
}

func (adapter *LogrusEntryAdapter) Warn(args ...interface{}) {
	adapter.e.Warn(args...)
}

func (adapter *LogrusEntryAdapter) Warning(args ...interface{}) {
	adapter.e.Warning(args...)
}

func (adapter *LogrusEntryAdapter) Error(args ...interface{}) {
	adapter.e.Error(args...)
}

func (adapter *LogrusEntryAdapter) Fatal(args ...interface{}) {
	adapter.e.Fatal(args...)
}

func (adapter *LogrusEntryAdapter) Panic(args ...interface{}) {
	adapter.e.Panic(args...)
}

func (adapter *LogrusEntryAdapter) Debugf(format string, args ...interface{}) {
	adapter.e.Debugf(format, args...)
}

func (adapter *LogrusEntryAdapter) Infof(format string, args ...interface{}) {
	adapter.e.Infof(format, args...)
}

func (adapter *LogrusEntryAdapter) Printf(format string, args ...interface{}) {
	adapter.e.Printf(format, args)
}

func (adapter *LogrusEntryAdapter) Warnf(format string, args ...interface{}) {
	adapter.e.Warnf(format, args...)
}

func (adapter *LogrusEntryAdapter) Warningf(format string, args ...interface{}) {
	adapter.e.Warningf(format, args...)
}

func (adapter *LogrusEntryAdapter) Errorf(format string, args ...interface{}) {
	adapter.e.Errorf(format, args...)
}

func (adapter *LogrusEntryAdapter) Fatalf(format string, args ...interface{}) {
	adapter.e.Fatalf(format, args...)
}

func (adapter *LogrusEntryAdapter) Panicf(format string, args ...interface{}) {
	adapter.e.Panicf(format, args...)
}

func (adapter *LogrusEntryAdapter) Debugln(args ...interface{}) {
	adapter.e.Debugln(args...)
}

func (adapter *LogrusEntryAdapter) Infoln(args ...interface{}) {
	adapter.e.Infoln(args...)
}

func (adapter *LogrusEntryAdapter) Println(args ...interface{}) {
	adapter.e.Println(args...)
}

func (adapter *LogrusEntryAdapter) Warnln(args ...interface{}) {
	adapter.e.Warnln(args...)
}

func (adapter *LogrusEntryAdapter) Warningln(args ...interface{}) {
	adapter.e.Warningln(args...)
}

func (adapter *LogrusEntryAdapter) Errorln(args ...interface{}) {
	adapter.e.Errorln(args...)
}

func (adapter *LogrusEntryAdapter) Fatalln(args ...interface{}) {
	adapter.e.Fatalln(args...)
}

func (adapter *LogrusEntryAdapter) Panicln(args ...interface{}) {
	adapter.e.Panicln(args...)
}

type Logger interface {
	WithField(key string, value interface{}) GenericEntry
	WithFields(fields Fields) GenericEntry
	WithError(err error) GenericEntry

	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Printf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Panicf(format string, args ...interface{})

	Debug(args ...interface{})
	Info(args ...interface{})
	Print(args ...interface{})
	Warn(args ...interface{})
	Warning(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Panic(args ...interface{})

	Debugln(args ...interface{})
	Infoln(args ...interface{})
	Println(args ...interface{})
	Warnln(args ...interface{})
	Warningln(args ...interface{})
	Errorln(args ...interface{})
	Fatalln(args ...interface{})
	Panicln(args ...interface{})

	WithConn(conn net.Conn) GenericEntry
	Reopen() error
	GetLogDest() string
	SetLevel(level string)
	GetLevel() string
	IsDebug() bool
	AddHook(h GenericHook)
}

type LogrusLoggerAdapter struct {
	l LogrusLogger
}

func (adapter *LogrusLoggerAdapter) WithConn(conn net.Conn) GenericEntry {
	entry := new(LogrusEntryAdapter)
	entry.e = adapter.l.WithConn(conn)
	return entry
}

func (adapter *LogrusLoggerAdapter) Reopen() error {
	return adapter.l.Reopen()
}

func (adapter *LogrusLoggerAdapter) GetLogDest() string {
	return adapter.l.GetLogDest()
}

func (adapter *LogrusLoggerAdapter) SetLevel(level string) {
	adapter.l.SetLevel(level)
}

func (adapter *LogrusLoggerAdapter) GetLevel() string {
	return adapter.l.GetLevel()
}

func (adapter *LogrusLoggerAdapter) IsDebug() bool {
	return adapter.l.IsDebug()
}

func (adapter *LogrusLoggerAdapter) AddHook(h GenericHook) {
	adapter.AddHook(h)
}

func (adapter *LogrusLoggerAdapter) WithField(key string, value interface{}) GenericEntry {
	entry := new(LogrusEntryAdapter)
	entry.e = adapter.l.WithField(key, value)
	return entry
}

func (adapter *LogrusLoggerAdapter) WithFields(fields Fields) GenericEntry {
	f := make(logrus.Fields)
	for k, v := range fields {
		f[k] = v
	}
	entry := new(LogrusEntryAdapter)
	entry.e = adapter.l.WithFields(f)
	return entry
}

func (adapter *LogrusLoggerAdapter) WithError(err error) GenericEntry {
	entry := new(LogrusEntryAdapter)
	entry.e = adapter.l.WithError(err)
	return entry
}

func (adapter *LogrusLoggerAdapter) Debugf(format string, args ...interface{}) {
	adapter.l.Debugf(format, args...)
}

func (adapter *LogrusLoggerAdapter) Infof(format string, args ...interface{}) {
	adapter.l.Infof(format, args...)
}

func (adapter *LogrusLoggerAdapter) Printf(format string, args ...interface{}) {
	adapter.l.Printf(format, args...)
}

func (adapter *LogrusLoggerAdapter) Warnf(format string, args ...interface{}) {
	adapter.l.Warnf(format, args...)
}

func (adapter *LogrusLoggerAdapter) Warningf(format string, args ...interface{}) {
	adapter.l.Warningf(format, args...)
}

func (adapter *LogrusLoggerAdapter) Errorf(format string, args ...interface{}) {
	adapter.l.Errorf(format, args...)
}

func (adapter *LogrusLoggerAdapter) Fatalf(format string, args ...interface{}) {
	adapter.l.Fatalf(format, args...)
}

func (adapter *LogrusLoggerAdapter) Panicf(format string, args ...interface{}) {
	adapter.l.Panicf(format, args...)
}

func (adapter *LogrusLoggerAdapter) Debug(args ...interface{}) {
	adapter.l.Debug(args...)
}

func (adapter *LogrusLoggerAdapter) Info(args ...interface{}) {
	adapter.l.Info(args...)
}

func (adapter *LogrusLoggerAdapter) Print(args ...interface{}) {
	adapter.l.Print(args...)
}

func (adapter *LogrusLoggerAdapter) Warn(args ...interface{}) {
	adapter.l.Warn(args...)
}

func (adapter *LogrusLoggerAdapter) Warning(args ...interface{}) {
	adapter.l.Warning(args...)
}

func (adapter *LogrusLoggerAdapter) Error(args ...interface{}) {
	adapter.l.Error(args...)
}

func (adapter *LogrusLoggerAdapter) Fatal(args ...interface{}) {
	adapter.l.Fatal(args...)
}

func (adapter *LogrusLoggerAdapter) Panic(args ...interface{}) {
	adapter.l.Panic(args...)
}

func (adapter *LogrusLoggerAdapter) Debugln(args ...interface{}) {
	adapter.l.Debugln(args...)
}

func (adapter *LogrusLoggerAdapter) Infoln(args ...interface{}) {
	adapter.l.Infoln(args...)
}

func (adapter *LogrusLoggerAdapter) Println(args ...interface{}) {
	adapter.l.Println(args...)
}

func (adapter *LogrusLoggerAdapter) Warnln(args ...interface{}) {
	adapter.l.Warnln(args...)
}

func (adapter *LogrusLoggerAdapter) Warningln(args ...interface{}) {
	adapter.l.Warningln(args...)
}

func (adapter *LogrusLoggerAdapter) Errorln(args ...interface{}) {
	adapter.l.Errorln(args...)
}

func (adapter *LogrusLoggerAdapter) Fatalln(args ...interface{}) {
	adapter.l.Fatalln(args...)
}

func (adapter *LogrusLoggerAdapter) Panicln(args ...interface{}) {
	adapter.l.Panicln(args...)
}
