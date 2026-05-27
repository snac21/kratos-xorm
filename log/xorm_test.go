package log

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	klog "github.com/go-kratos/kratos/v2/log"
	"xorm.io/xorm/contexts"
	xormlog "xorm.io/xorm/log"
)

func TestNewXormLoggerEnablesSQLAtDebugLevel(t *testing.T) {
	logger := NewXormLogger(klog.NewStdLogger(io.Discard), "debug")
	if logger.Level() != xormlog.LOG_DEBUG {
		t.Fatalf("expected debug level, got %v", logger.Level())
	}
	if !logger.IsShowSQL() {
		t.Fatal("expected debug level to enable sql logging")
	}
}

func TestNewXormLoggerDisablesSQLAtInfoLevel(t *testing.T) {
	logger := NewXormLogger(klog.NewStdLogger(io.Discard), "info")
	if logger.Level() != xormlog.LOG_OFF {
		t.Fatalf("expected off level, got %v", logger.Level())
	}
	if logger.IsShowSQL() {
		t.Fatal("expected info level to disable sql logging")
	}
}

func TestXormLoggerShowSQLCanBeOverridden(t *testing.T) {
	logger := NewXormLogger(klog.NewStdLogger(io.Discard), "info")
	logger.ShowSQL(true)
	if !logger.IsShowSQL() {
		t.Fatal("expected ShowSQL(true) to enable sql logging")
	}
}

func TestXormLoggerAfterSQLCompactsWhitespace(t *testing.T) {
	var buf bytes.Buffer
	logger := NewXormLogger(klog.NewStdLogger(&buf), "debug")

	logger.AfterSQL(xormlog.LogContext(contexts.ContextHook{
		Ctx: context.Background(),
		SQL: `
SELECT DISTINCT
  rm.menu_id
FROM user_role ur
INNER JOIN role_menu rm ON rm.role_id = ur.role_id
WHERE ur.user_id = ?
ORDER BY rm.menu_id ASC
`,
		Args:        []interface{}{int64(1)},
		ExecuteTime: 5 * time.Millisecond,
	}))

	got := buf.String()
	if strings.Contains(got, "\nSELECT DISTINCT") || strings.Contains(got, "\\nSELECT DISTINCT") {
		t.Fatalf("expected sql log to be compacted into one line, got %q", got)
	}
	if !strings.Contains(got, "SELECT DISTINCT rm.menu_id FROM user_role ur INNER JOIN role_menu rm ON rm.role_id = ur.role_id WHERE ur.user_id = ? ORDER BY rm.menu_id ASC") {
		t.Fatalf("expected compacted sql in log, got %q", got)
	}
	if strings.Contains(got, "caller=log/xorm.go") || strings.Contains(got, "caller=xorm@") {
		t.Fatalf("expected caller to skip xorm adapter and xorm internals, got %q", got)
	}
}

func TestShortCaller(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		line     int
		expected string
	}{
		{
			name:     "无路径分隔符文件名",
			file:     "auth.go",
			line:     108,
			expected: "auth.go:108",
		},
		{
			name:     "单级目录路径",
			file:     "/auth.go",
			line:     108,
			expected: "auth.go:108",
		},
		{
			name:     "标准两级目录路径",
			file:     "data/auth.go",
			line:     108,
			expected: "auth.go:108",
		},
		{
			name:     "多级长路径截取",
			file:     "/Users/Cage/snac21/github-go/xorm-learn/bss-v2/internal/data/auth.go",
			line:     108,
			expected: "data/auth.go:108",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortCaller(tt.file, tt.line)
			if got != tt.expected {
				t.Errorf("shortCaller() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestShouldSkipSQLCallerFrame(t *testing.T) {
	tests := []struct {
		name     string
		function string
		file     string
		expected bool
	}{
		{
			name:     "Go 运行时框架函数应跳过",
			function: "runtime.goexit",
			file:     "/usr/local/go/src/runtime/asm_amd64.s",
			expected: true,
		},
		{
			name:     "测试框架函数应跳过",
			function: "testing.tRunner",
			file:     "/usr/local/go/src/testing/testing.go",
			expected: true,
		},
		{
			name:     "Kratos 框架内部帧应跳过",
			function: "github.com/go-kratos/kratos/v2/log.(*Helper).Log",
			file:     "/pkg/mod/github.com/go-kratos/kratos/v2@v2.7.0/log/helper.go",
			expected: true,
		},
		{
			name:     "xorm 内部实现帧应跳过",
			function: "xorm.io/xorm/(*Session).Exec",
			file:     "/pkg/mod/xorm.io/xorm@v1.3.11/session_raw.go",
			expected: true,
		},
		{
			name:     "当前适配层文件自身应跳过",
			function: "github.com/snac21/kratos-xorm/log.resolveSQLCaller",
			file:     "/Users/Cage/.../kratos-xorm/log/xorm.go",
			expected: true,
		},
		{
			name:     "业务仓储代码不应跳过",
			function: "bss/internal/data.(*authRepo).UpdateLastAccess",
			file:     "/Users/Cage/.../internal/data/auth.go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipSQLCallerFrame(tt.function, tt.file)
			if got != tt.expected {
				t.Errorf("shouldSkipSQLCallerFrame() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestIsDataCallerFrame(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		expected bool
	}{
		{
			name:     "非 internal/data 路径",
			file:     "/Users/Cage/snac21/github-go/xorm-learn/bss-v2/internal/service/auth.go",
			expected: false,
		},
		{
			name:     "正规 internal/data 路径",
			file:     "/Users/Cage/snac21/github-go/xorm-learn/bss-v2/internal/data/auth.go",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDataCallerFrame(tt.file)
			if got != tt.expected {
				t.Errorf("isDataCallerFrame() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestIsPreferredDataCallerFrame(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		expected bool
	}{
		{
			name:     "非 data 层文件",
			file:     "/Users/Cage/snac21/github-go/xorm-learn/bss-v2/internal/service/auth.go",
			expected: false,
		},
		{
			name:     "data 层初始化文件应排除",
			file:     "/Users/Cage/snac21/github-go/xorm-learn/bss-v2/internal/data/data.go",
			expected: false,
		},
		{
			name:     "具体的业务 Repository 文件应优先包含",
			file:     "/Users/Cage/snac21/github-go/xorm-learn/bss-v2/internal/data/auth.go",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPreferredDataCallerFrame(tt.file)
			if got != tt.expected {
				t.Errorf("isPreferredDataCallerFrame() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
