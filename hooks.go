package audit

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"github.com/qustavo/sqlhooks/v2"
)

// Hooks satisfies the sqlhook.Hooks interface
type Hooks struct {
	Auditor *Auditor
}

var (
	ErrInvalidDatabaseDriver = fmt.Errorf("invalid database driver")
	ErrNoAuditSet            = fmt.Errorf("no audit is set from the request context")
)

func RegisterHooks(auditor *Auditor, dbType string) (string, error) {
	databaseDriverName := fmt.Sprintf("store-hooks-%s", dbType)

	hooks := &Hooks{
		Auditor: auditor,
	}

	switch dbType {
	case MysqlDB:
		auditor.dbType = MysqlDB
		sql.Register(databaseDriverName, sqlhooks.Wrap(&mysql.MySQLDriver{}, hooks))
	case PostgresDB:
		auditor.dbType = PostgresDB
		sql.Register(databaseDriverName, sqlhooks.Wrap(&pq.Driver{}, hooks))

	default:
		return "invalid_driver", ErrInvalidDatabaseDriver
	}

	return databaseDriverName, nil
}

func (h *Hooks) Before(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
	name, err := h.Auditor.GetTableName(query)
	if err != nil {
		return nil, err
	}

	var event Event

	isExempted := isExempted(h.Auditor.tableException, name)
	if err != nil {
		return ctx, err
	}
	event.IsExempted = isExempted

	if !isExempted {
		ev, ok := ctx.Value("audit").(Event)
		if !ok {
			return nil, ErrNoAuditSet
		}
		ev.IsExempted = isExempted

		ev, err = h.Auditor.SetEvent(ctx, ev, name, query, args)
		if err != nil {
			return nil, err
		}
		return context.WithValue(ctx, "audit", ev), nil
	}

	return context.WithValue(ctx, "audit", event), nil
}

func (h *Hooks) After(ctx context.Context, result driver.Result, rows driver.Rows, query string, args ...interface{}) (context.Context, error) {
	ev := ctx.Value("audit").(Event)

	var lastInsertID int64
	if !ev.IsExempted {
		if ev.Action == "insert" {
			switch h.Auditor.dbType {
			case MysqlDB:
				id, err := result.LastInsertId()
				if err != nil {
					return ctx, err
				}
				lastInsertID = id
			case PostgresDB:
				lastInsertID = 0
			}
		}

		err := h.Auditor.Save(ctx, query, args, lastInsertID, ev)
		if err != nil {
			return nil, err
		}
	}

	return ctx, nil
}
