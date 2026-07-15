package kratosxorm

import (
	"context"

	"xorm.io/xorm"
)

// UpdateStruct is an alias of map[string]any, representing a collection of fields and
// their corresponding values to be updated in a database record.
//
// Using this alias improves code readability and clearly expresses the developer's
// intent when performing partial updates on a model.
// 1. Define the fields and values you want to update
// updateData := xrom.UpdateStruct{
//     "status":      "active",
//     "updated_at":  time.Now(),
//     "login_count": 10,
// }

// 2. Pass it to your update method
// db.Model(&User{}).Where("id = ?", 1).Updates(updateData)
type UpdateStruct map[string]any

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
