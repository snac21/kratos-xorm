package kratosxorm

import (
	"testing"

	"xorm.io/builder"
)

func TestQueryPageBuilderSQL(t *testing.T) {
	stmt := builder.MySQL().Select("id", "name").From("user").Where(builder.Eq{"status": 1})
	countStmt := builder.MySQL().Select("COUNT(1) AS total").From(stmt, "temp_count")

	sqlText, args, err := countStmt.ToSQL()
	if err != nil {
		t.Fatalf("failed to generate count sql: %v", err)
	}

	expectedSQL := "SELECT COUNT(1) AS total FROM (SELECT id,name FROM user WHERE status=?) temp_count"
	if sqlText != expectedSQL {
		t.Errorf("expected SQL %q, got %q", expectedSQL, sqlText)
	}

	if len(args) != 1 || args[0] != 1 {
		t.Errorf("expected args [1], got %v", args)
	}
}
