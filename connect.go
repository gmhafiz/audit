package audit

import (
	"context"
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

type Auditor struct {
	dbType DBType
	db     *sqlx.DB
	Event  Event
}

const CreateDB = `CREATE TABLE IF NOT EXISTS audits;`
var (
	InsertDB = "INSERT INTO audits (`organisation_id` ,`action`, `actor_id`, `old_values`, `new_values`, `url`, `ip_address`, `user_agent`, `created_at`,`updated_at`, `deleted_at`) VALUES(?,?,?,?,?,?,?,?,?,?,?);}
)

type DBType string

const (
	Mysql    DBType = "mysql"
	Postgres DBType = "postgres"
)

func newMysqlAuditor(db *sql.DB) *Auditor {
	// todo: default prefix is 'audit'. Can be customized
	// todo: check if table exists, if not create

	auditor := &Auditor{
		dbType: Mysql,
		db:     sqlx.NewDb(db, string(Mysql)),
	}

	return auditor
}

func newPostgresAuditor(db *sql.DB) *Auditor {
	// todo: default prefix is 'audit'. Can be customized
	// todo: check if table exists, if not create

	auditor := &Auditor{
		dbType: Postgres,
		db:     sqlx.NewDb(db, string(Postgres)),
	}

	return auditor
}


func (a *Auditor) SetEvent(event *Event) {
	a.Event = *event
}

func (a *Auditor) Save(ctx context.Context) error {
	switch a.dbType {
	case Mysql:
		_, err := a.db.ExecContext(ctx, InsertDB)
		return err
	case Postgres:
		_, err := a.db.ExecContext(ctx, InsertDB)
		return err
	default:
		return  ErrInvalidConnection
	}
}
