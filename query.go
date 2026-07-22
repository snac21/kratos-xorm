package kratosxorm

import (
	"context"
	"database/sql"

	"xorm.io/builder"
	"xorm.io/xorm"
)

type CountRow struct {
	Total int64 `xorm:"'total'"`
}

// TenantScope 标识实体需要注入租户隔离
type TenantScope interface {
	TenantIdField() string // 返回租户 ID 的 SQL 字段名，例如 "tenant_id"
}

// TenantEvaluator 全局租户上下文解析器接口，由外部模块启动时注册
type TenantEvaluator interface {
	Evaluate(ctx context.Context) (tenantID int64, isPlatformAdmin bool, ok bool)
}

// ActiveTenantEvaluator 当前激活的租户评估器实例
var ActiveTenantEvaluator TenantEvaluator

// ApplyTenantScope 自动检测并为 xorm builder 注入租户条件限制
func ApplyTenantScope(ctx context.Context, stmt *builder.Builder, fieldName string) *builder.Builder {
	if ActiveTenantEvaluator == nil {
		return stmt
	}
	tenantID, isPlatformAdmin, ok := ActiveTenantEvaluator.Evaluate(ctx)
	if !ok || isPlatformAdmin {
		return stmt
	}
	// 注入租户隔离条件
	return stmt.And(builder.Eq{fieldName: tenantID})
}

// QueryOneByBuilder executes a builder-generated single-table query and scans one row.
func QueryOneByBuilder[T any](ctx context.Context, session *xorm.Session, stmt *builder.Builder) (*T, bool, error) {
	var t T
	if ts, ok := any(t).(TenantScope); ok {
		stmt = ApplyTenantScope(ctx, stmt, ts.TenantIdField())
	}

	sqlText, args, err := stmt.ToSQL()
	if err != nil {
		return nil, false, err
	}

	row := new(T)
	has, err := session.SQL(sqlText, args...).Get(row)
	if err != nil {
		return nil, false, err
	}
	if !has {
		return nil, false, nil
	}
	return row, true, nil
}

// QueryListByBuilder executes a builder-generated single-table query and scans rows.
func QueryListByBuilder[T any](ctx context.Context, session *xorm.Session, stmt *builder.Builder) ([]T, error) {
	var t T
	if ts, ok := any(t).(TenantScope); ok {
		stmt = ApplyTenantScope(ctx, stmt, ts.TenantIdField())
	}

	sqlText, args, err := stmt.ToSQL()
	if err != nil {
		return nil, err
	}

	rows := make([]T, 0)
	if err := session.SQL(sqlText, args...).Find(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// QueryListBySQL executes a raw SQL query and scans rows for multi-table reads.
func QueryListBySQL[T any](session *xorm.Session, sqlText string, args ...interface{}) ([]T, error) {
	rows := make([]T, 0)
	if err := session.SQL(sqlText, args...).Find(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// CountByBuilder executes a builder-generated COUNT query and returns the scalar count.
func CountByBuilder(ctx context.Context, session *xorm.Session, stmt *builder.Builder) (int64, error) {
	row, has, err := QueryOneByBuilder[CountRow](ctx, session, stmt)
	if err != nil {
		return 0, err
	}
	if !has {
		return 0, nil
	}
	return row.Total, nil
}

// ExecBuilder executes a builder-generated single-table write statement.
func ExecBuilder(ctx context.Context, session *xorm.Session, stmt *builder.Builder) (sql.Result, error) {
	sqlText, args, err := stmt.ToSQL()
	if err != nil {
		return nil, err
	}
	sqlOrArgs := make([]interface{}, 0, len(args)+1)
	sqlOrArgs = append(sqlOrArgs, sqlText)
	sqlOrArgs = append(sqlOrArgs, args...)
	return session.Exec(sqlOrArgs...)
}

// Page defines a generic paginated response structure.
type Page[T any] struct {
	Current int64 `json:"current"`
	Size    int64 `json:"size"`
	Total   int64 `json:"total"`
	Records []T   `json:"records"`
}

// QueryPageBuilder executes a generic paginated query.
func QueryPageBuilder[T any](ctx context.Context, session *xorm.Session, stmt *builder.Builder, current int64, size int64) (*Page[T], error) {
	var t T
	if ts, ok := any(t).(TenantScope); ok {
		stmt = ApplyTenantScope(ctx, stmt, ts.TenantIdField())
	}

	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}

	// 1. 构造通用的子查询 COUNT 语句以获取总数。
	// 这可以保证无论原始 stmt 包含何种 select 列、JOIN 或复杂的 WHERE，都能正确统计出总记录数。
	countStmt := builder.Select("COUNT(1) AS total").From(stmt, "temp_count")
	total, err := CountByBuilder(ctx, session, countStmt)
	if err != nil {
		return nil, err
	}

	if total == 0 {
		return &Page[T]{
			Current: current,
			Size:    size,
			Total:   0,
			Records: make([]T, 0),
		}, nil
	}

	// 2. 执行分页查询
	limit := int(size)
	offset := int((current - 1) * size)

	// 使用 builder 原生的 Limit 方法，自动适配数据库方言。
	stmt.Limit(limit, offset)
	sqlText, args, err := stmt.ToSQL()
	if err != nil {
		return nil, err
	}

	records := make([]T, 0)
	err = session.SQL(sqlText, args...).Find(&records)
	if err != nil {
		return nil, err
	}

	return &Page[T]{
		Current: current,
		Size:    size,
		Total:   total,
		Records: records,
	}, nil
}
