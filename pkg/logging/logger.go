package logging

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	defaultLogLevel = "info"
)

type Field struct {
	zapField zap.Field
}

type Logger struct {
	base *zap.Logger
}

func String(key, val string) Field {
	return Field{zapField: zap.String(key, val)}
}

func Int(key string, val int) Field {
	return Field{zapField: zap.Int(key, val)}
}

func Int64(key string, val int64) Field {
	return Field{zapField: zap.Int64(key, val)}
}

func Bool(key string, val bool) Field {
	return Field{zapField: zap.Bool(key, val)}
}

func Error(err error) Field {
	return Field{zapField: zap.Error(err)}
}

func NamedError(key string, err error) Field {
	return Field{zapField: zap.NamedError(key, err)}
}

func Any(key string, val any) Field {
	return Field{zapField: zap.Any(key, val)}
}

// ParseLevel 解析日志级别，支持 debug/info/warn/error。
func ParseLevel(raw string) (zapcore.Level, error) {
	level := strings.ToLower(strings.TrimSpace(raw))
	switch level {
	case "debug":
		return zap.DebugLevel, nil
	case "info":
		return zap.InfoLevel, nil
	case "warn":
		return zap.WarnLevel, nil
	case "error":
		return zap.ErrorLevel, nil
	default:
		return zap.InfoLevel, fmt.Errorf("invalid log_level %q: must be one of debug|info|warn|error", raw)
	}
}

// Init 初始化全局 logger，输出固定为 JSON 到 stdout。
func Init(level string) error {
	if strings.TrimSpace(level) == "" {
		level = defaultLogLevel
	}
	logger, err := newLogger(level, zapcore.AddSync(os.Stdout))
	if err != nil {
		return err
	}
	zap.ReplaceGlobals(logger)
	return nil
}

// L 返回附带 module 字段的 logger。
func L(module string) *Logger {
	if strings.TrimSpace(module) == "" {
		return &Logger{base: zap.L()}
	}
	return &Logger{base: zap.L().With(zap.String("module", module))}
}

func (l *Logger) With(fields ...Field) *Logger {
	return &Logger{base: l.unwrap().With(toZapFields(fields)...)}
}

func (l *Logger) Debug(msg string, fields ...Field) {
	l.unwrap().Debug(msg, toZapFields(fields)...)
}

func (l *Logger) Info(msg string, fields ...Field) {
	l.unwrap().Info(msg, toZapFields(fields)...)
}

func (l *Logger) Warn(msg string, fields ...Field) {
	l.unwrap().Warn(msg, toZapFields(fields)...)
}

func (l *Logger) Error(msg string, fields ...Field) {
	l.unwrap().Error(msg, toZapFields(fields)...)
}

// Sync 刷新 logger 缓冲；忽略 stdout/stderr 的常见无效 sync 错误。
func Sync() {
	err := zap.L().Sync()
	if err == nil {
		return
	}
	if isIgnorableSyncError(err) {
		return
	}
}

func newLogger(level string, sink zapcore.WriteSyncer) (*zap.Logger, error) {
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339NanoTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), sink, parsed)
	return zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)), nil
}

func (l *Logger) unwrap() *zap.Logger {
	if l == nil || l.base == nil {
		return zap.L()
	}
	return l.base
}

func toZapFields(fields []Field) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]zap.Field, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.zapField)
	}
	return out
}

func isIgnorableSyncError(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		msg := strings.ToLower(e.Error())
		if strings.Contains(msg, "invalid argument") ||
			strings.Contains(msg, "inappropriate ioctl for device") ||
			strings.Contains(msg, "bad file descriptor") {
			return true
		}
	}
	return false
}
