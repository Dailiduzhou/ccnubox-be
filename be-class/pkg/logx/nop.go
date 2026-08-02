package logx

import (
	"context"

	"github.com/asynccnu/ccnubox-be/common/pkg/logger"
)

type nopLogger struct{}

func Nop() logger.Logger                                    { return nopLogger{} }
func (nopLogger) WithContext(context.Context) logger.Logger { return nopLogger{} }
func (nopLogger) With(...logger.Field) logger.Logger        { return nopLogger{} }
func (nopLogger) Debug(string, ...logger.Field)             {}
func (nopLogger) Info(string, ...logger.Field)              {}
func (nopLogger) Warn(string, ...logger.Field)              {}
func (nopLogger) Error(string, ...logger.Field)             {}
func (nopLogger) Debugf(string, ...interface{})             {}
func (nopLogger) Infof(string, ...interface{})              {}
func (nopLogger) Warnf(string, ...interface{})              {}
func (nopLogger) Errorf(string, ...interface{})             {}
func (nopLogger) AddCallerSkip(int) logger.Logger           { return nopLogger{} }
