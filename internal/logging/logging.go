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

func Logger() *zap.SugaredLogger {
	if logger == nil {
		panic("logger not set up")
	}
	return logger
}
