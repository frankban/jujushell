package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/errgo.v1"
)

// Logger is used to log messages for the shell server.
var logger *zap.SugaredLogger

// Setup sets up logging at the given level.
func Setup(level zapcore.Level) error {
	cfg := zap.NewProductionConfig()
	cfg.Level.SetLevel(level)
	log, err := cfg.Build()
	if err != nil {
		return errgo.Mask(err)
	}
	logger = log.Sugar()
	return nil
}

// Logger returns a logger. It sets the logger up if not done yet.
func Logger() *zap.SugaredLogger {
	if logger == nil {
		Setup(zapcore.InfoLevel)
	}
	return logger
}
