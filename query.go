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
