package main

import (
	"context"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/datawire/dlib/dlog"
	"github.com/telepresenceio/telepresence/v2/pkg/log"
)

func makeBaseLogger(ctx context.Context) context.Context {
	logrusLogger := logrus.New()
	logrusFormatter := log.NewFormatter("2006-01-02 15:04:05.0000")
	logrusLogger.SetFormatter(logrusFormatter)

	log.SetLogrusLevel(logrusLogger, os.Getenv("LOG_LEVEL"))

	logger := dlog.WrapLogrus(logrusLogger)
	dlog.SetFallbackLogger(logger)
	ctx = dlog.WithLogger(ctx, logger)
	return log.WithLevelSetter(ctx, logrusLogger)
}
