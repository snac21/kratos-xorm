package kratosxorm

import (
	"database/sql"

	"xorm.io/builder"
	"xorm.io/xorm"
)

type CountRow struct {
	Total int64 `xorm:"'total'"`
}

// QueryOneByBuilder executes a builder-generated single-table query and scans one row.
func QueryOneByBuilder[T any](session *xorm.Session, stmt *builder.Builder) (*T, bool, error) {
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
func QueryListByBuilder[T any](session *xorm.Session, stmt *builder.Builder) ([]T, error) {
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
func CountByBuilder(session *xorm.Session, stmt *builder.Builder) (int64, error) {
	row, has, err := QueryOneByBuilder[CountRow](session, stmt)
	if err != nil {
		return 0, err
	}
	if !has {
		return 0, nil
	}
	return row.Total, nil
}

// ExecBuilder executes a builder-generated single-table write statement.
func ExecBuilder(session *xorm.Session, stmt *builder.Builder) (sql.Result, error) {
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
func QueryPageBuilder[T any](session *xorm.Session, stmt *builder.Builder, current int64, size int64) (*Page[T], error) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}

	// 1. 构造通用的子查询 COUNT 语句以获取总数。
	// 这可以保证无论原始 stmt 包含何种 select 列、JOIN 或复杂的 WHERE，都能正确统计出总记录数。
	countStmt := builder.MySQL().Select("COUNT(1) AS total").From(stmt, "temp_count")
	total, err := CountByBuilder(session, countStmt)
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

	// 2. 执行分页查询。
	limit := int(size)
	offset := int((current - 1) * size)

	sqlText, args, err := stmt.ToSQL()
	if err != nil {
		return nil, err
	}

	records := make([]T, 0)
	err = session.Limit(limit, offset).SQL(sqlText, args...).Find(&records)
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

