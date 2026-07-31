package logger

import (
	"time"
)

type LogLevel int

const (
	Silent LogLevel = iota
	Error
	Warn
	Info
)

type Config struct {
	SlowThreshold time.Duration
	LogLevel      LogLevel
	Colorful      bool
}

type Interface interface {
	LogMode(LogLevel) Interface
}

type defaultLogger struct{}

func (d *defaultLogger) LogMode(l LogLevel) Interface {
	return d
}

var Default Interface = &defaultLogger{}
