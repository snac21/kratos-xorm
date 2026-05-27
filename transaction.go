package kratosxorm

import (
	"context"

	"xorm.io/xorm"
)

// TxFunc defines reusable transaction work.
type TxFunc func(session *xorm.Session) error

// ExecTx executes business logic in a transaction template.
func ExecTx(engine *xorm.Engine, ctx context.Context, fn TxFunc) error {
	session := engine.NewSession()
	defer session.Close()
	session = session.Context(ctx)

	if err := session.Begin(); err != nil {
		return err
	}
	if err := fn(session); err != nil {
		_ = session.Rollback()
		return err
	}
	if err := session.Commit(); err != nil {
		_ = session.Rollback()
		return err
	}
	return nil
}
