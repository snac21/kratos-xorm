package log

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	klog "github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	xlog "xorm.io/xorm/log"
)

// xormLogger 将项目内统一的 Kratos Logger 适配成 xorm 可识别的日志接口。
// 这里额外维护了 xorm 的日志级别和 showSQL 开关，避免 xorm 使用默认 stdout logger。
type xormLogger struct {
	logger  klog.Logger
	helper  *klog.Helper
	level   xlog.LogLevel
	showSQL bool
}

var _ xlog.ContextLogger = (*xormLogger)(nil)

// NewXormLogger 创建 xorm 专用日志器。
// 这里不复用应用层默认 caller，而是交给 AfterSQL 自己回溯栈帧，确保 caller 落到 data 层。
func NewXormLogger(base klog.Logger, showSQL bool) xlog.ContextLogger {
	var level xlog.LogLevel
	if showSQL {
		level = xlog.LOG_DEBUG
	} else {
		level = xlog.LOG_OFF
	}
	logger := klog.With(
		base,
		"component", "xorm",
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)
	return &xormLogger{
		logger:  logger,
		helper:  klog.NewHelper(logger),
		level:   level,
		showSQL: showSQL,
	}
}

// Debug 族方法直接透传给项目日志器，并沿用 xorm 自己的日志级别判断。
func (l *xormLogger) Debug(v ...any) {
	if l.level <= xlog.LOG_DEBUG {
		l.helper.Debug(v...)
	}
}

func (l *xormLogger) Debugf(format string, v ...any) {
	if l.level <= xlog.LOG_DEBUG {
		l.helper.Debugf(format, v...)
	}
}

// Error 族方法用于输出 xorm 运行期错误，例如 SQL 执行异常。
func (l *xormLogger) Error(v ...any) {
	if l.level <= xlog.LOG_ERR {
		l.helper.Error(v...)
	}
}

func (l *xormLogger) Errorf(format string, v ...any) {
	if l.level <= xlog.LOG_ERR {
		l.helper.Errorf(format, v...)
	}
}

// Info 族方法用于输出普通的 xorm 信息，包括 SQL 执行日志。
func (l *xormLogger) Info(v ...any) {
	if l.level <= xlog.LOG_INFO {
		l.helper.Info(v...)
	}
}

func (l *xormLogger) Infof(format string, v ...any) {
	if l.level <= xlog.LOG_INFO {
		l.helper.Infof(format, v...)
	}
}

// Warn 族方法用于输出 xorm 警告日志。
func (l *xormLogger) Warn(v ...any) {
	if l.level <= xlog.LOG_WARNING {
		l.helper.Warn(v...)
	}
}

func (l *xormLogger) Warnf(format string, v ...any) {
	if l.level <= xlog.LOG_WARNING {
		l.helper.Warnf(format, v...)
	}
}

// Level 返回当前 xorm logger 的日志级别。
func (l *xormLogger) Level() xlog.LogLevel {
	return l.level
}

// SetLevel 允许 xorm 在运行期动态调整日志级别。
func (l *xormLogger) SetLevel(level xlog.LogLevel) {
	l.level = level
}

// ShowSQL 控制是否打印 SQL。
// xorm 内部会通过这个开关决定是否调用 BeforeSQL/AfterSQL。
func (l *xormLogger) ShowSQL(show ...bool) {
	if len(show) == 0 {
		l.showSQL = true
		return
	}
	l.showSQL = show[0]
}

// IsShowSQL 返回当前 SQL 打印开关状态。
func (l *xormLogger) IsShowSQL() bool {
	return l.showSQL
}

// BeforeSQL 是 xorm.ContextLogger 必需方法。
// 当前不在 SQL 执行前打日志，所以保持空实现。
func (l *xormLogger) BeforeSQL(_ xlog.LogContext) {}

// AfterSQL 在 SQL 执行完成后输出日志。
// 这里做两件事：
// 1. 把多行 SQL 压成单行，避免 JSON 日志中出现大量 \n 转义字符。
// 2. 重新计算 caller，让日志定位到 internal/data 下真正发起 SQL 的仓储代码位置。
func (l *xormLogger) AfterSQL(ctx xlog.LogContext) {
	if !l.showSQL {
		return
	}
	// 压缩 SQL，避免原始多行字符串污染日志输出。
	sqlText := compactSQL(ctx.SQL)
	logger := klog.WithContext(ctx.Ctx, l.logger)
	caller := resolveSQLCaller()
	if ctx.Err != nil {
		_ = logger.Log(klog.LevelError, "caller", caller, klog.DefaultMessageKey, fmt.Sprintf("[SQL] %s %v - %v - err=%v", sqlText, ctx.Args, ctx.ExecuteTime, ctx.Err))
		return
	}
	if ctx.ExecuteTime > 0 {
		_ = logger.Log(klog.LevelInfo, "caller", caller, klog.DefaultMessageKey, fmt.Sprintf("[SQL] %s %v - %v", sqlText, ctx.Args, ctx.ExecuteTime))
		return
	}
	_ = logger.Log(klog.LevelInfo, "caller", caller, klog.DefaultMessageKey, fmt.Sprintf("[SQL] %s %v", sqlText, ctx.Args))
}

// compactSQL 将 SQL 中连续空白字符折叠成一个空格，便于日志阅读和检索。
func compactSQL(sqlText string) string {
	fields := strings.Fields(sqlText)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

// resolveSQLCaller 扫描当前调用栈，选出最合适的业务 caller。
// 目标不是返回 xorm 框架内部位置，而是返回 data 层仓储文件位置，便于定位真实 SQL 发起处。
//
// 典型调用链：
//
//	data/auth.go (业务仓储) → kratosxorm.QueryOneByBuilder (通用查询) → xorm/session.go → xorm/logger → AfterSQL → resolveSQLCaller
//
// 我们需要跳过 xorm 和日志适配器内部帧，精确定位到 data/auth.go。
func resolveSQLCaller() string {
	// 最大回溯深度，20 层足以完整覆盖 xorm 内部调用栈 + 仓储层调用栈
	const maxDepth = 20
	// 极致优化：使用固定大小的栈分配数组代替切片创建，完全消除堆逃逸 and 内存分配开销
	var pcs [maxDepth]uintptr
	// 捕获调用栈，skip=3 表示越过：runtime.Callers 自身、resolveSQLCaller、AfterSQL
	n := runtime.Callers(3, pcs[:])
	if n == 0 {
		return "unknown:0"
	}

	// 转换为可处理内联函数展开的迭代器 Frames
	frames := runtime.CallersFrames(pcs[:n])

	// fallbackData 记录 data 层基础设施文件的 caller（优先级 2，如 data.go）
	var fallbackData string
	// fallbackExternal 记录第一个非框架外部帧的 caller（优先级 3）
	var fallbackExternal string

	// 使用 Frames 迭代器双向零分配遍历，找到高优帧即刻短路返回，保障极致性能
	for {
		frame, more := frames.Next()

		// 1. 跳过框架内部实现帧（runtime, testing, kratos, xorm 以及当前公共库自身）
		if shouldSkipSQLCallerFrame(frame.Function, frame.File) {
			if !more {
				break
			}
			continue
		}

		// 2. 如果是仓储层具体业务文件（优先级最高），短路直接返回
		if isPreferredDataCallerFrame(frame.File) {
			return shortCaller(frame.File, frame.Line)
		}

		// 3. 如果是仓储层公共基础设施（如 data.go），记录为次优候选（仅保留第一条）
		if isDataCallerFrame(frame.File) && fallbackData == "" {
			fallbackData = shortCaller(frame.File, frame.Line)
			if !more {
				break
			}
			continue
		}

		// 4. 其余外部调用帧，记录为兜底候选（仅保留第一条）
		if fallbackExternal == "" {
			fallbackExternal = shortCaller(frame.File, frame.Line)
		}

		if !more {
			break
		}
	}

	// 按优先级降级返回最优解析结果
	if fallbackData != "" {
		return fallbackData
	}
	if fallbackExternal != "" {
		return fallbackExternal
	}
	return "unknown:0"
}

// shouldSkipSQLCallerFrame 过滤掉不应该作为业务 caller 的内部栈帧。
// 判断逻辑：
//   - 函数名以框架前缀或当前库前缀开头 → 跳过
func shouldSkipSQLCallerFrame(function string, file string) bool {
	// 需要跳过的函数名前缀列表
	skipPrefixes := []string{
		"runtime.",                       // Go 运行时
		"testing.",                       // 测试框架
		"github.com/go-kratos/kratos/",   // Kratos 框架内部
		"xorm.io/xorm/",                  // xorm 内部
		"github.com/snac21/kratos-xorm/", // 迁移后的公共库所有包前缀
	}
	// 用函数全限定名做前缀匹配，比文件路径 Contains 更精确
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(function, prefix) {
			return true
		}
	}
	return false
}

// isDataCallerFrame 判断当前帧是否位于 data 层。
// 通过文件路径中包含 "/internal/data/" 来识别。
func isDataCallerFrame(file string) bool {
	return strings.Contains(file, "/internal/data/")
}

// isPreferredDataCallerFrame 判断当前帧是否是优先返回的 data 层业务文件。
// 排除 data.go 这类通用基础设施文件，
// 因为它们是多个仓储方法共用的中间层，指向它们的 caller 没有区分度。
func isPreferredDataCallerFrame(file string) bool {
	// 前提：必须先在 data 层内
	if !isDataCallerFrame(file) {
		return false
	}
	// data 层中需要排除的基础设施文件
	skipSuffixes := []string{
		"/internal/data/data.go", // 数据层初始化和依赖注入
	}
	// 如果文件是上述基础设施文件之一，返回 false，降级为次优候选
	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(file, suffix) {
			return false
		}
	}
	// 不在排除列表中的 data 层文件，即为具体仓储文件（如 auth.go、role.go）
	return true
}

// shortCaller 把绝对路径压缩成类似 data/auth.go:108 的短格式，便于日志展示。
func shortCaller(file string, line int) string {
	// 找到最后一个 '/'，即文件名分隔符
	last := strings.LastIndexByte(file, '/')
	if last == -1 {
		// 没有路径分隔符（不太可能），直接拼接文件名和行号
		return file + ":" + strconv.Itoa(line)
	}
	// 找到倒数第二个 '/'，用于截取两级目录
	prev := strings.LastIndexByte(file[:last], '/')
	if prev == -1 {
		// 只有一级目录，返回文件名部分
		return file[last+1:] + ":" + strconv.Itoa(line)
	}
	// 截取 prev+1 到末尾，得到 "data/auth.go"，再拼上行号
	return file[prev+1:] + ":" + strconv.Itoa(line)
}
