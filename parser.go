package audit

import (
	"context"
)

type Parser struct {
	*MysqlParser
	*PostgresParser

	parserType string
}

func NewParser(dbType string) *Parser {
	return &Parser{
		parserType: dbType,
	}
}

type Work interface {
	setEvent(ctx context.Context, s store, name string, event Event, tableName, query string, args []interface{}) (Event, error)
	getOldValues(ctx context.Context, db store, auditTableName WhereClause, tableName string, query string, args []interface{}) (output string, w WhereClause, err error)
	runQuery(ctx context.Context, s store, auditTableName WhereClause, tableName, query string, args []interface{}) (out []byte, w WhereClause, err error)
	getNameAndWherePosition(s string) (name string, position int, err error)
	queryMarshal(ctx context.Context, s store, ww WhereClause) ([]byte, uint64, error)
	Save(ctx context.Context, query string, args []interface{}, lastInsertID int64, event Event) error
}
