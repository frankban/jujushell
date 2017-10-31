package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// logger is used to log messages for the shell server.
var logger *Logger

// Logger is the logger used by the applocation.
type Logger struct {
	*zap.SugaredLogger
	config zap.Config
}

// Log returns the logger. It sets the logger up if not done yet.
func Log() *Logger {
	if logger != nil {
		return logger
	}
	logger = &Logger{
		config: zap.NewProductionConfig(),
	}
	log, err := logger.config.Build()
	if err != nil {
		// This should never happen.
		panic(err)
	}
	logger.SugaredLogger = log.Sugar()
	return logger
}

// SetLevel sets up logging at the given level.
func (l *Logger) SetLevel(level zapcore.Level) {
	l.config.Level.SetLevel(level)
}
