package audit

import (
	"database/sql"
	"fmt"
	"time"
)

var (
	ErrInvalidConnection = fmt.Errorf("invalid database connection")
)

type Action string

const (
	Read   Action = "Read"
	Create        = "Create"
	Update        = "Update"
	Delete        = "Delete"
)

type Event struct {
	Organisation uint64     `db:"organisation"` // or tenant
	ActorID      uint64     `db:"actor_id"`
	Table        string     `db:"table"`
	Action       Action     `db:"action"`
	OldValues    string     `db:"old_values"`
	NewValues    string     `db:"new_values"`
	HTTPMethod   string     `db:"http_method"`
	URL          string     `db:"url"`
	IPAddress    string  `db:"ip_address"`
	UserAgent    string     `db:"user_agent"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	DeletedAt    time.Time `db:"deleted_at"`
}

//type Interceptor struct {
//	Auditor
//	Event Event
//}

func NewAudit(db *sql.DB, driver string) (*Auditor, error) {
	switch driver {
	case "mysql":
		newMysqlAuditor(db)
	case "postgres":
		newPostgresAuditor(db)
	default:
		return nil, ErrInvalidConnection
	}
	return &Auditor{}, nil
}

//var _ Interceptor = AuditInterceptor{}
