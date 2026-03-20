package logging

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
)

const (
	defaultLogLevel = "info"
)

type Field struct {
	zapField zap.Field
}

type Logger struct {
	base   *zap.Logger
	module string
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

// Init 初始化全局 logger，输出为 zap console/development 风格文本到 stdout。
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

// L 返回附带 module 消息前缀的 logger。
func L(module string) *Logger {
	return &Logger{base: zap.L(), module: strings.TrimSpace(module)}
}

func (l *Logger) With(fields ...Field) *Logger {
	if l == nil {
		return &Logger{base: zap.L().With(toZapFields(fields)...)}
	}
	return &Logger{base: l.unwrap().With(toZapFields(fields)...), module: l.module}
}

func (l *Logger) Debug(msg string, fields ...Field) {
	l.unwrap().Debug(l.formatMessage(msg), toZapFields(fields)...)
}

func (l *Logger) Info(msg string, fields ...Field) {
	l.unwrap().Info(l.formatMessage(msg), toZapFields(fields)...)
}

func (l *Logger) Warn(msg string, fields ...Field) {
	l.unwrap().Warn(l.formatMessage(msg), toZapFields(fields)...)
}

func (l *Logger) Error(msg string, fields ...Field) {
	l.unwrap().Error(l.formatMessage(msg), toZapFields(fields)...)
}

// Sync 刷新 logger 缓冲；忽略 stdout/stderr 的常见无效 sync 错误。
func Sync() error {
	err := zap.L().Sync()
	if err == nil {
		return nil
	}
	if isIgnorableSyncError(err) {
		return nil
	}
	return err
}

func newLogger(level string, sink zapcore.WriteSyncer) (*zap.Logger, error) {
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	encoderConfig := zap.NewDevelopmentEncoderConfig()
	if shouldColorizeConsole() {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), sink, parsed)
	return zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)), nil
}

func (l *Logger) unwrap() *zap.Logger {
	if l == nil || l.base == nil {
		return zap.L()
	}
	return l.base
}

func (l *Logger) formatMessage(msg string) string {
	if l == nil || l.module == "" {
		return msg
	}
	if msg == "" {
		return fmt.Sprintf("[%s]", l.module)
	}
	return fmt.Sprintf("[%s] %s", l.module, msg)
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
	leafErrors := collectLeafErrors(err)
	if len(leafErrors) == 0 {
		return false
	}
	for _, e := range leafErrors {
		if !isIgnorableSyncErrorMessage(e) {
			return false
		}
	}
	return true
}

func collectLeafErrors(err error) []error {
	if err == nil {
		return nil
	}

	const maxTraverseNodes = 1024
	visitedPointers := make(map[uintptr]struct{})
	stack := []error{err}
	leaves := make([]error, 0, 1)
	processed := 0

	for len(stack) > 0 {
		processed++
		if processed > maxTraverseNodes {
			return nil
		}

		n := len(stack) - 1
		cur := stack[n]
		stack = stack[:n]

		if cur == nil {
			continue
		}
		if ptr, ok := errorPointer(cur); ok {
			if _, seen := visitedPointers[ptr]; seen {
				continue
			}
			visitedPointers[ptr] = struct{}{}
		}

		children := unwrapErrors(cur)
		if len(children) == 0 {
			leaves = append(leaves, cur)
			continue
		}
		for _, child := range children {
			if child != nil {
				stack = append(stack, child)
			}
		}
	}

	return leaves
}

func errorPointer(err error) (uintptr, bool) {
	v := reflect.ValueOf(err)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return 0, false
	}
	return v.Pointer(), true
}

func unwrapErrors(err error) []error {
	type unwrapOne interface {
		Unwrap() error
	}
	type unwrapMany interface {
		Unwrap() []error
	}

	if uw, ok := err.(unwrapMany); ok {
		return uw.Unwrap()
	}
	if uw, ok := err.(unwrapOne); ok {
		if child := uw.Unwrap(); child != nil {
			return []error{child}
		}
	}
	return nil
}

func isIgnorableSyncErrorMessage(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "inappropriate ioctl for device") ||
		strings.Contains(msg, "bad file descriptor")
}

func shouldColorizeConsole() bool {
	executablePath, _ := os.Executable()
	inputs := colorizeConsoleInputs{
		forceColor:         envBool("SIMICLAW_FORCE_COLOR"),
		noColor:            envBool("SIMICLAW_NO_COLOR"),
		stdoutTTY:          term.IsTerminal(int(os.Stdout.Fd())),
		stderrTTY:          term.IsTerminal(int(os.Stderr.Fd())),
		jetbrainsRunBinary: looksLikeJetBrainsRunBinary(executablePath),
		runningTests:       flag.Lookup("test.v") != nil,
	}
	return shouldColorizeConsoleFor(inputs)
}

type colorizeConsoleInputs struct {
	forceColor         bool
	noColor            bool
	stdoutTTY          bool
	stderrTTY          bool
	jetbrainsRunBinary bool
	runningTests       bool
}

func shouldColorizeConsoleFor(inputs colorizeConsoleInputs) bool {
	if inputs.forceColor {
		return true
	}
	if inputs.noColor {
		return false
	}
	if inputs.stdoutTTY || inputs.stderrTTY {
		return true
	}
	if inputs.runningTests {
		return false
	}
	return inputs.jetbrainsRunBinary
}

func looksLikeJetBrainsRunBinary(executablePath string) bool {
	normalized := strings.TrimSpace(filepath.ToSlash(executablePath))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "/JetBrains/") &&
		(strings.Contains(normalized, "/tmp/GoLand/") || strings.Contains(normalized, "/tmp/IntelliJIdea/"))
}

func envBool(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return false
	}
	switch strings.ToLower(value) {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}
