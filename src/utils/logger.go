// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package utils

import (
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	stdoutLogger    *zap.SugaredLogger
	stderrorLogger  *zap.SugaredLogger
	stacktraceLevel *zap.AtomicLevel
	logLevel        *zap.AtomicLevel
}

func NewLogger() *Logger {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
		fmt.Fprintf(os.Stderr, "Hostname retrieval error.: %s\n", err.Error())
	}

	stacktraceLevel := zap.NewAtomicLevelAt(zap.DPanicLevel)
	logLevel := zap.NewAtomicLevelAt(zap.InfoLevel)

	stderrBWS := &zapcore.BufferedWriteSyncer{
		WS:            zapcore.AddSync(os.Stderr),
		Size:          512 * 1024,
		FlushInterval: 500 * time.Millisecond,
	}
	stdoutBWS := &zapcore.BufferedWriteSyncer{
		WS:            zapcore.AddSync(os.Stdout),
		Size:          512 * 1024,
		FlushInterval: 500 * time.Millisecond,
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	encoder := zapcore.NewConsoleEncoder(encCfg)

	opts := []zap.Option{
		zap.WithCaller(true),
		zap.ErrorOutput(stderrBWS),
		zap.AddStacktrace(&stacktraceLevel),
		zap.AddCallerSkip(1),
	}

	logger := &Logger{
		stdoutLogger:    zap.New(zapcore.NewCore(encoder, stdoutBWS, &logLevel), opts...).Sugar().Named(hostname),
		stderrorLogger:  zap.New(zapcore.NewCore(encoder, stderrBWS, &logLevel), opts...).Sugar().Named(hostname),
		stacktraceLevel: &stacktraceLevel,
		logLevel:        &logLevel,
	}

	return logger
}

func (logger *Logger) SetVerbose(isVerbose bool) {
	if isVerbose {
		logger.logLevel.SetLevel(zap.DebugLevel)
	} else {
		logger.logLevel.SetLevel(zap.InfoLevel)
	}
}

func (logger *Logger) Close() {
	logger.stdoutLogger.Sync()
	logger.stderrorLogger.Sync()
}

var loggerInstance *Logger
var loggerOnce = &sync.Once{}

func GetLogger() *Logger {
	loggerOnce.Do(func() {
		loggerInstance = NewLogger()
	})
	return loggerInstance
}

func DebugPrintf(format string, args ...interface{}) {
	GetLogger().stdoutLogger.Debugf(format, args...)
}

func ErrorPrintf(format string, args ...interface{}) {
	GetLogger().stderrorLogger.Errorf(format, args...)
}

func Printf(format string, args ...interface{}) {
	GetLogger().stdoutLogger.Infof(format, args...)
}
