# kratos-xorm

`kratos-xorm` 是一个为 Go-Kratos 和 XORM 框架提供集成与通用基础设施的 Go 库。

## 功能特性

- **日志适配器**：将 Kratos 的 Logger 适配为 XORM 的 ContextLogger，支持 SQL 压缩打印、TraceID/SpanID 追踪及智能 caller 回溯跳过。
- **泛型查询与分页封装**：提供基于 `xorm.io/builder` 的泛型单表/分页查询及 Scalar 数量统计封装（如 `QueryOneByBuilder`、`QueryPageBuilder` 等）。
- **租户行级自动隔离**：通过 `TenantScope` 和 `TenantEvaluator` 接口实现数据库单表及分页查询中无侵入式自动租户 ID 注入。
- **事务模板**：提供无侵入式的事务处理模板 `ExecTx`。

## 安装

```bash
go get github.com/snac21/kratos-xorm
```

## 快速使用

### 1. 自动租户数据行隔离插件

#### A. 实现 `TenantScope` 接口标记行隔离实体
为需要进行租户隔离的 Xorm 模型实现 `TenantScope`，指定数据库表中的租户 ID 字段名：

```go
import "github.com/snac21/kratos-xorm"

type User struct {
    Id       int64  `xorm:"'id'"`
    TenantId int32  `xorm:"'tenant_id'"`
    Username string `xorm:"'username'"`
}

// 开启行隔离并指定租户字段为 tenant_id
func (User) TenantIdField() string {
    return "tenant_id"
}
```

#### B. 实现并挂载全局租户评估器 `TenantEvaluator` 接口
在数据层或者引导入口中，实现 `TenantEvaluator` 接口，用于从 context 动态提取当前用户的租户信息，并将其挂载到 `ActiveTenantEvaluator`：

```go
import (
    "context"
    kratosxorm "github.com/snac21/kratos-xorm"
)

type tenantEvaluatorImpl struct{}

func (e *tenantEvaluatorImpl) Evaluate(ctx context.Context) (tenantID int64, isPlatformAdmin bool, ok bool) {
    // 示例：从 context 中解析当前登录人
    if identity, ok := FromContext(ctx); ok {
        return identity.TenantID, identity.IsPlatformAdmin, true
    }
    return 0, false, false
}

// 注册激活评估器
kratosxorm.ActiveTenantEvaluator = &tenantEvaluatorImpl{}
```

在执行 `QueryOneByBuilder`、`QueryListByBuilder` 或 `QueryPageBuilder` 时，`kratos-xorm` 会自动调用 `ActiveTenantEvaluator` 校验租户并在 Xorm Builder 中安全地追加租户条件隔离。

---

### 2. 日志适配器初始化

```go
import (
    kratosxormlog "github.com/snac21/kratos-xorm/log"
)

// 创建并设置日志适配器
xormLogger := kratosxormlog.NewXormLogger(kratosLogger, "debug")
engine.SetLogger(xormLogger)
engine.ShowSQL(true)
```

---

### 3. 泛型查询

```go
import (
    kratosxorm "github.com/snac21/kratos-xorm"
    "xorm.io/builder"
)

// 单条查询
user, has, err := kratosxorm.QueryOneByBuilder[User](ctx, session, builder.MySQL().Select("*").From("user").Where(builder.Eq{"id": 1}))
```

---

### 4. 事务模板

```go
import (
    kratosxorm "github.com/snac21/kratos-xorm"
)

err := kratosxorm.ExecTx(engine, ctx, func(session *xorm.Session) error {
    // 执行事务操作
    return nil
})
```
