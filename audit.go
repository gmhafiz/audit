package audit

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidConnection = fmt.Errorf("invalid database connection")
)

type Action string

const (
	Select Action = "select"
	Insert Action = "insert"
	Update Action = "update"
	Delete Action = "delete"
)

type Event struct {
	ActorID    uint64    `db:"actor_id"`
	TableRowID uint64    `db:"table_row_id"`
	Table      string    `db:"table_name"`
	Action     Action    `db:"action"`
	OldValues  string    `db:"old_values"`
	NewValues  string    `db:"new_values"`
	HTTPMethod string    `db:"http_method"`
	URL        string    `db:"url"`
	IPAddress  string    `db:"ip_address"`
	UserAgent  string    `db:"user_agent"`
	CreatedAt  time.Time `db:"created_at"`

	WhereClause WhereClause
	IsExempted  bool
}

type Option func(*Auditor)

var defaultAuditor = &Auditor{
	auditTableName: "audits",
	tableException: []string{"audits"},
}

// NewAudit created a new auditor instance
func NewAudit(opts ...Option) (*Auditor, error) {
	a := defaultAuditor
	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}

// WithTableName customise the audit table name
func WithTableName(tableName string) Option {
	return func(a *Auditor) {
		sanitised, err := sanitise(tableName)
		if err != nil {
			log.Fatalln(err)
		}
		a.auditTableName = sanitised
		a.tableException = append(a.tableException, tableName)
	}
}

//func WithMongo(dsn string) Option {
//	return func(a *Auditor) {
//		client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(dsn))
//		if err != nil {
//			log.Fatal(err)
//		}
//		a.store.mongo = client
//	}
//}
//
//// WithQueue todo: Allow client to use existing queue to save into database
//func WithQueue() Option {
//	return func(a *Auditor) {
//
//	}
//}

// WithTableException list of tables not to be audited
func WithTableException(tableNames ...string) Option {
	exceptions := make([]string, 0)
	for _, name := range tableNames {
		exceptions = append(exceptions, strings.ToLower(name))
	}
	return func(a *Auditor) {
		a.tableException = append(a.tableException, exceptions...)
	}
}

func isExempted(exception []string, tableName string) bool {
	if tableName == "" {
		return true
	}
	for _, table := range exception {
		if tableName == table {
			return true
		}
	}

	return false
}

func sanitise(name string) (string, error) {
	return onlyAlphaNumeric(name)
}

func onlyAlphaNumeric(name string) (string, error) {
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		return "", err
	}
	processedString := reg.ReplaceAllString(name, "")

	return processedString, nil
}
