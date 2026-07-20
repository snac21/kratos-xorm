package log

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	xlog "xorm.io/xorm/log"
)

// xormLogger 将项目内统一的 Logger 适配成 xorm 可识别的日志接口。
type xormLogger struct {
	logger  *slog.Logger
	showSQL atomic.Bool // 原子布尔值，防止并发 SetLevel / ShowSQL 产生 Data Race
	// selfPackageName 保存适配器自身的包全名（例如 "base-line/pkg/log" 或 "github.com/snac21/kratos-xorm/v3/log"）。
	// 用于在扫描 SQL 调用栈时，自适应、动态地跳过适配层自身的所有代码帧，防止 caller 误标为 xorm.go。
	selfPackageName string
}

var _ xlog.ContextLogger = (*xormLogger)(nil)

// NewXormLogger 创建 xorm 专用日志器。
// 第二个参数 showSQLOrConfig 支持直传 bool (如 true/false)、string (如 "debug")、或包含 Level 字段的配置结构体/指针。
func NewXormLogger(base *slog.Logger, showSQLOrConfig any) xlog.ContextLogger {
	showSQL := resolveShowSQL(showSQLOrConfig)
	logger := base.With(
		slog.String("component", "xorm"),
	)

	// 惰性显式解析适配器自身的包路径全称，自适应任意项目/包名
	var selfPkg string
	if pc, _, _, ok := runtime.Caller(0); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			fullName := fn.Name()
			if idx := strings.LastIndex(fullName, "."); idx != -1 {
				selfPkg = fullName[:idx]
			}
		}
	}

	l := &xormLogger{
		logger:          logger,
		selfPackageName: selfPkg,
	}
	l.showSQL.Store(showSQL)
	return l
}

// resolveShowSQL 辅助函数：自动智能解析 bool、string 级别或 Config 配置结构体
func resolveShowSQL(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.EqualFold(strings.TrimSpace(val), "debug")
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return false
		}
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		field := val.FieldByName("Level")
		if field.IsValid() && field.Kind() == reflect.String {
			return strings.EqualFold(strings.TrimSpace(field.String()), "debug")
		}
	}

	return false
}

// formatMsg 优化变长参数的格式化，对于单 string 入参避免 fmt.Sprint 的反射与堆内存分配
func formatMsg(v ...any) string {
	if len(v) == 1 {
		if str, ok := v[0].(string); ok {
			return str
		}
	}
	return fmt.Sprint(v...)
}

// logToSlogContext 统一将 xorm 日志格式化并通过底层 slog.Handler 进行分发，注入 Context 以支持链路追踪。
func (l *xormLogger) logToSlogContext(ctx context.Context, level slog.Level, msg string) {
	var pcs [1]uintptr
	// skip=3 代表越过：runtime.Callers, logToSlogContext, 以及调用 logToSlogContext 的接口方法
	runtime.Callers(3, pcs[:])
	pc := pcs[0]

	handler := l.logger.Handler()
	if ctx == nil {
		ctx = context.Background()
	}
	if !handler.Enabled(ctx, level) {
		return
	}

	record := slog.NewRecord(time.Now(), level, msg, pc)
	_ = handler.Handle(ctx, record)
}

// Debug 族方法直接透传给项目日志器，当且仅当 showSQL 为真时输出。
func (l *xormLogger) Debug(v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelDebug, formatMsg(v...))
	}
}

func (l *xormLogger) Debugf(format string, v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelDebug, fmt.Sprintf(format, v...))
	}
}

// Error 族方法用于输出 xorm 运行期错误，例如 SQL 执行异常。
func (l *xormLogger) Error(v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelError, formatMsg(v...))
	}
}

func (l *xormLogger) Errorf(format string, v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelError, fmt.Sprintf(format, v...))
	}
}

// Info 族方法用于输出普通的 xorm 信息，包括 SQL 执行日志。
func (l *xormLogger) Info(v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelInfo, formatMsg(v...))
	}
}

func (l *xormLogger) Infof(format string, v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelInfo, fmt.Sprintf(format, v...))
	}
}

// Warn 族方法用于输出 xorm 警告日志。
func (l *xormLogger) Warn(v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelWarn, formatMsg(v...))
	}
}

func (l *xormLogger) Warnf(format string, v ...any) {
	if l.showSQL.Load() {
		l.logToSlogContext(nil, slog.LevelWarn, fmt.Sprintf(format, v...))
	}
}

// Level 根据 showSQL 返回适配的日志级别。
func (l *xormLogger) Level() xlog.LogLevel {
	if l.showSQL.Load() {
		return xlog.LOG_DEBUG
	}
	return xlog.LOG_OFF
}

// SetLevel 根据设置的日志级别动态更新 showSQL 状态，使用原子操作控制。
func (l *xormLogger) SetLevel(level xlog.LogLevel) {
	l.showSQL.Store(level <= xlog.LOG_DEBUG)
}

// ShowSQL 控制是否打印 SQL。
func (l *xormLogger) ShowSQL(show ...bool) {
	if len(show) == 0 {
		l.showSQL.Store(true)
		return
	}
	l.showSQL.Store(show[0])
}

// IsShowSQL 返回当前 SQL 打印开关状态。
func (l *xormLogger) IsShowSQL() bool {
	return l.showSQL.Load()
}

// BeforeSQL 是 xorm.ContextLogger 必需方法。
func (l *xormLogger) BeforeSQL(_ xlog.LogContext) {}

// AfterSQL 在 SQL 执行完成后输出日志，直接解析并注入真正的业务 PC 帧。
func (l *xormLogger) AfterSQL(ctx xlog.LogContext) {
	if !l.showSQL.Load() {
		return
	}
	sqlText := compactSQL(ctx.SQL)
	pc := l.resolveSQLCallerPC()

	var level slog.Level
	var msg string
	if ctx.Err != nil {
		level = slog.LevelError
		msg = fmt.Sprintf("[SQL] %s %v - %v - err=%v", sqlText, ctx.Args, ctx.ExecuteTime, ctx.Err)
	} else if ctx.ExecuteTime > 0 {
		level = slog.LevelInfo
		msg = fmt.Sprintf("[SQL] %s %v - %v", sqlText, ctx.Args, ctx.ExecuteTime)
	} else {
		level = slog.LevelInfo
		msg = fmt.Sprintf("[SQL] %s %v", sqlText, ctx.Args)
	}

	handler := l.logger.Handler()
	// 这里传入的是从业务层传递下来的真实 ctx.Ctx (包含 OpenTelemetry TraceContext)
	if !handler.Enabled(ctx.Ctx, level) {
		return
	}

	record := slog.NewRecord(time.Now(), level, msg, pc)
	// 这里直接将真实的 ctx.Ctx 传给了 slog.Handler 物理落盘！
	_ = handler.Handle(ctx.Ctx, record)
}

// compactSQL 将 SQL 中连续空白字符折叠成一个空格。增加短路判断，避免无换行 SQL 触发无谓内存分配。
func compactSQL(sqlText string) string {
	if !strings.Contains(sqlText, "\n") && !strings.Contains(sqlText, "\r") {
		return strings.TrimSpace(sqlText)
	}
	fields := strings.Fields(sqlText)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

// resolveSQLCallerPC 扫描当前调用栈，选出最合适的业务物理 PC，让 slog 直接获得准确的文件名和行号。
//
//go:noinline
func (l *xormLogger) resolveSQLCallerPC() uintptr {
	const maxDepth = 25
	var pcs [maxDepth]uintptr
	n := runtime.Callers(3, pcs[:])
	if n == 0 {
		return 0
	}

	var fallbackDataPC uintptr
	var fallbackExternalPC uintptr

	for i := range n {
		pc := pcs[i]
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		funcName := fn.Name()
		if l.shouldSkipSQLCallerFrame(funcName) {
			continue
		}

		file, _ := fn.FileLine(pc)

		// 1. 如果是具体的业务 Repository 文件 (排除 data.go 与基础库文件)，直接短路返回物理 PC
		if isPreferredDataCallerFrame(file) {
			return pc
		}

		// 2. 如果是 data 层基础设施 (如 data.go)，记录为次优候选
		if isDataCallerFrame(file) && fallbackDataPC == 0 {
			fallbackDataPC = pc
		}

		// 3. 其余外部调用帧，记录为兜底候选
		if fallbackExternalPC == 0 {
			fallbackExternalPC = pc
		}
	}

	if fallbackDataPC != 0 {
		return fallbackDataPC
	}
	return fallbackExternalPC
}

// shouldSkipSQLCallerFrame 过滤掉不应该作为业务 caller 的内部栈帧。
func (l *xormLogger) shouldSkipSQLCallerFrame(function string) bool {
	if l.selfPackageName != "" && strings.HasPrefix(function, l.selfPackageName) {
		return true
	}

	skipPrefixes := []string{
		"runtime.",                    // Go 运行时
		"testing.",                    // 测试框架
		"database/sql",                // Go 标准库数据库驱动
		"github.com/go-kratos/kratos", // Kratos 框架内部
		"xorm.io/xorm",                // xorm 内部
	}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(function, prefix) {
			return true
		}
	}
	return false
}

// isDataCallerFrame 判断当前帧是否位于 data 层。
func isDataCallerFrame(file string) bool {
	return strings.Contains(file, "/internal/data/")
}

// isPreferredDataCallerFrame 判断当前帧是否是优先返回的 data 层业务文件。
func isPreferredDataCallerFrame(file string) bool {
	if !isDataCallerFrame(file) {
		return false
	}
	return !strings.HasSuffix(file, "/internal/data/data.go")
}
