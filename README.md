# kratos-xorm

`kratos-xorm` 是一个为 Go-Kratos 和 XORM 框架提供集成与通用基础设施的 Go 库。

## 功能特性

- **日志适配器**：将 Kratos 的 Logger 适配为 XORM 的 ContextLogger，支持 SQL 压缩打印、TraceID/SpanID 追踪及智能 caller 回溯跳过。
- **泛型查询封装**：提供基于 `xorm.io/builder` 的泛型单表查询及 Scalar 数量统计封装（如 `QueryOneByBuilder`、`QueryListByBuilder` 等）。
- **事务模板**：提供无侵入式的事务处理模板 `ExecTx`。

## 安装

```bash
go get github.com/snac21/kratos-xorm
```

## 快速使用

### 1. 日志适配器初始化

```go
import (
    kratosxormlog "github.com/snac21/kratos-xorm/log"
)

// 创建并设置日志适配器
xormLogger := kratosxormlog.NewXormLogger(kratosLogger, "debug")
engine.SetLogger(xormLogger)
engine.ShowSQL(true)
```

### 2. 泛型查询

```go
import (
    kratosxorm "github.com/snac21/kratos-xorm"
    "xorm.io/builder"
)

// 单条查询
user, has, err := kratosxorm.QueryOneByBuilder[User](session, builder.MySQL().Select("*").From("user").Where(builder.Eq{"id": 1}))
```

### 3. 事务模板

```go
import (
    kratosxorm "github.com/snac21/kratos-xorm"
)

err := kratosxorm.ExecTx(engine, ctx, func(session *xorm.Session) error {
    // 执行事务操作
    return nil
})
```
