package logger

import (
	"fmt"
	"io"
	"os"
)

// Logger takes in a message and tag pairs.
type Logger interface {
	Debug(msg string, tags ...interface{})
	Info(msg string, tags ...interface{})
	Warn(msg string, tags ...interface{})
	Error(msg string, tags ...interface{})
}

type logger struct{ out io.Writer }

// New creates a logger that writes to stdout.
func New() Logger { return &logger{os.Stdout} }

func (l *logger) print(msg string, tags ...interface{}) {
	fmt.Fprintln(l.out, append([]interface{}{msg}, tags...))
}

// Debug creates a debug log entry.
func (l *logger) Debug(msg string, tags ...interface{}) { l.print(msg, tags...) }

// Info creates an info log entry.
func (l *logger) Info(msg string, tags ...interface{}) { l.print(msg, tags...) }

// Warn creates a warn log entry.
func (l *logger) Warn(msg string, tags ...interface{}) { l.print(msg, tags...) }

// Error creates an error log entry.
func (l *logger) Error(msg string, tags ...interface{}) { l.print(msg, tags...) }
