package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/blastrain/vitess-sqlparser/sqlparser"
	_ "github.com/go-sql-driver/mysql"
)

type MysqlParser struct {
	db       *sql.DB
	query    query
	internal *sql.DB
	parsed   sqlparser.Statement
}

func (p *MysqlParser) getTableName(query string) (tableName string, err error) {
	return getSqlTableName(query)
}

func getSqlTableName(query string) (tableName string, err error) {
	query = strings.ToLower(query)

	firstSyntax := getSqlAction(query)

	switch firstSyntax {
	case "update":
		tableName = After(query, "update")
	case "insert":
		index := regexp.MustCompile("insert(.*?)into").FindStringIndex(query)
		tableName = after(query, index[1])
	case "delete":
		index := regexp.MustCompile("delete(.*?)from").FindStringIndex(query)
		tableName = after(query, index[1])
	}

	if tableName == "" {
		return "", nil
	}

	return strings.ToLower(strings.Trim(tableName, "`")), nil
}

func getSqlAction(query string) string {
	return strings.ToLower(query[:strings.IndexRune(query, ' ')])
}

func (p *MysqlParser) setEvent(ctx context.Context, s store, auditTableName string, event Event, tableName, query string, args []interface{}) (Event, error) {
	action := getSqlAction(query)

	ww := WhereClause{}

	ww.tableName = tableName
	sqlAction := getSqlAction(query)

	if ww.tableName == auditTableName {
		return Event{}, nil
	}

	oldValues, w, err := p.getOldValues(ctx, s, ww, sqlAction, query, args)
	if err != nil {
		return event, err
	}

	switch action {
	case string(Select):
		fallthrough
	case string(Update):
		fallthrough
	case string(Insert):
		fallthrough
	case string(Delete):
		event.Action = Action(action)
		event.TableRowID = w.id
		event.Table = tableName
		event.OldValues = oldValues
		event.WhereClause = w
		return event, nil
	default:
		return Event{}, ErrInvalidQuery
	}
}

func (p *MysqlParser) getOldValues(ctx context.Context, db store, ww WhereClause, sqlAction string, query string, args []interface{}) (output string, w WhereClause, err error) {
	oldValues, w, err := p.runQuery(ctx, db, ww, sqlAction, query, args)
	if err != nil {
		return "", WhereClause{}, err
	}

	return string(oldValues), w, nil
}

func (p *MysqlParser) runQuery(ctx context.Context, s store, ww WhereClause, sqlAction, query string, args []interface{}) (out []byte, w WhereClause, err error) {
	// todo: support 'IN' operator
	// todo: support key-value store

	tree, err := sqlparser.Parse(query)
	if err != nil {
		return nil, ww, err
	}
	p.parsed = tree

	var name string
	var position int
	buf := sqlparser.NewTrackedBuffer(nil)
	switch sqlAction {
	case string(Update):
		sel := tree.(*sqlparser.Update)
		ww.operator = sel.Where.Expr.(*sqlparser.ComparisonExpr).Operator
		expr := tree.(*sqlparser.Update).Where.Expr
		expr.Format(buf)
	case string(Delete):
		sel := tree.(*sqlparser.Delete)
		ww.operator = sel.Where.Expr.(*sqlparser.ComparisonExpr).Operator
		expr := tree.(*sqlparser.Delete).Where.Expr
		expr.Format(buf)
	default:
		return []byte("{}"), ww, nil
	}

	name, position, err = p.getNameAndWherePosition(buf.String())
	if err != nil {
		return nil, ww, err
	}
	ww.col = name
	ww.val = args[position-1]

	marshalled, affectedID, err := p.queryMarshal(ctx, s, ww)
	if err != nil {
		return nil, WhereClause{}, err
	}
	ww.id = affectedID
	if err != nil {
		return nil, ww, err
	}

	return marshalled, ww, nil
}

func (p *MysqlParser) getNameAndWherePosition(s string) (name string, position int, err error) {
	splits := strings.Split(s, " ")
	named := splits[2]

	position, err = strconv.Atoi(named[2:])

	name = splits[0]

	return name, position, err
}

func (p *MysqlParser) queryMarshal(ctx context.Context, s store, ww WhereClause) ([]byte, uint64, error) {
	var affectedID uint64 // todo: id can be a string

	query := fmt.Sprintf(p.query.selectStmt, ww.tableName, ww.col, ww.operator)
	rows, err := s.sql.QueryContext(ctx, query, ww.val)
	if err != nil {
		return nil, affectedID, err
	}
	cols, err := rows.Columns()
	if err != nil {
		return nil, affectedID, err
	}
	vals := make([]interface{}, len(cols))
	toString := make(map[string]string, len(cols))
	for i := range cols {
		vals[i] = new(sql.RawBytes)
	}
	for rows.Next() {
		_ = rows.Scan(vals...)
	}

	for i, val := range vals {
		content := reflect.ValueOf(val).Interface().(*sql.RawBytes)
		toString[cols[i]] = string(*content)
		if cols[i] == "id" { // todo: customise table id name
			affectedID, err = strconv.ParseUint(toString[cols[i]], 10, 64)
			if err != nil {
				return nil, 0, err
			}
		}
	}

	marshaled, err := json.Marshal(toString)
	if err != nil {
		return nil, affectedID, err
	}
	return marshaled, affectedID, nil
}

func (p *MysqlParser) Save(ctx context.Context, query string, args []interface{}, lastInsertID int64, event Event) error {
	var e error
	query = strings.ToLower(query)

	switch event.Action {
	case Insert:
		event = p.setNewInsertValues(ctx, event, lastInsertID, query, args)

		_, err := p.internal.ExecContext(ctx, p.query.insert,
			event.ActorID,
			event.TableRowID,
			event.Table,
			event.Action,
			event.OldValues,
			event.NewValues,
			event.HTTPMethod,
			event.URL,
			event.IPAddress,
			event.UserAgent,
			event.CreatedAt,
		)
		e = err
	case Update:
		event = p.setNewUpdateValues(ctx, event, query, args)

		_, err := p.internal.ExecContext(ctx, p.query.insert,
			event.ActorID,
			event.TableRowID,
			event.Table,
			event.Action,
			event.OldValues,
			event.NewValues,
			event.HTTPMethod,
			event.URL,
			event.IPAddress,
			event.UserAgent,
			event.CreatedAt,
		)
		e = err
	case Select:
		e = nil
	case Delete:
		event.NewValues = "{}"
		event.CreatedAt = time.Now()
		_, err := p.internal.ExecContext(ctx, p.query.insert,
			event.ActorID,
			event.TableRowID,
			event.Table,
			event.Action,
			event.OldValues,
			event.NewValues,
			event.HTTPMethod,
			event.URL,
			event.IPAddress,
			event.UserAgent,
			event.CreatedAt,
		)
		e = err
	default:
		e = ErrInvalidConnection
	}

	return e
}

func (p *MysqlParser) setNewInsertValues(ctx context.Context, event Event, lastInsertID int64, query string, args []interface{}) Event {
	marshalled, err := p.marshalInsertQuery(query, lastInsertID, args)
	if err != nil {
		return Event{}
	}
	event.TableRowID = uint64(lastInsertID)
	event.NewValues = string(marshalled)
	event.CreatedAt = time.Now()

	return event
}

func (p *MysqlParser) setNewUpdateValues(ctx context.Context, event Event, query string, args []interface{}) Event {
	newValues, err := p.marshallFromUpdateQueryArgs(query, args)
	if err != nil {
		return event
	}
	event.NewValues = string(newValues)
	event.CreatedAt = time.Now()

	val, ok := ctx.Value("userID").(uint64)
	if ok {
		event.ActorID = val
	}

	return event
}

func (p *MysqlParser) marshalInsertQuery(query string, lastInsertID int64, args []interface{}) ([]byte, error) {
	toString := make(map[string]interface{}, len(args)+1)

	columnNames := getColumnNamesFromInsert(query)
	for i, col := range columnNames {
		toString[col] = args[i]
	}

	toString["id"] = lastInsertID

	marshalled, err := json.Marshal(toString)
	if err != nil {
		return nil, err
	}

	return marshalled, nil
}

func (p *MysqlParser) marshallFromUpdateQueryArgs(query string, args []interface{}) ([]byte, error) {
	sel := p.parsed.(*sqlparser.Update)

	buf := sqlparser.NewTrackedBuffer(nil)
	sel.Format(buf)
	name, position, err := getNameAndWherePositionForUpdate(buf.String())
	if err != nil {
		return nil, err
	}
	wc := WhereClause{
		col:      name,
		operator: sel.Where.Expr.(*sqlparser.ComparisonExpr).Operator,
		val:      args[position-1],
	}

	toString := make(map[string]interface{}, len(args)+1)

	columnNames := getColumnNamesFromUpdate(query)
	for i, col := range columnNames {
		col = strings.ReplaceAll(col, "`", "")
		toString[col] = args[i]
	}
	toString[wc.col] = wc.val

	marshalled, err := json.Marshal(toString)
	if err != nil {
		return nil, err
	}

	return marshalled, nil
}

func After(query, word string) string {
	iWord := strings.Index(query, strings.ToLower(word)) + len(word) + 1
	return after(query, iWord)
}

func after(query string, iWord int) (atAfter string) {
	iAfter := 0

	for i := iWord; i < len(query); i++ {
		r := rune(query[i])
		if unicode.IsLetter(r) && iAfter <= 0 {
			iAfter = i
		}

		if (unicode.IsSpace(r) || unicode.IsPunct(r)) && iAfter > 0 {
			atAfter = query[iAfter:i]
			break
		}
	}

	if atAfter == "" {
		atAfter = query[iAfter:]
	}

	return
}
