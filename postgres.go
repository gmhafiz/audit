package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	pg_query "github.com/pganalyze/pg_query_go/v2"
)

type PostgresParser struct {
	db       *sql.DB
	query    query
	internal *sql.DB
	parsed   string
}

func (p *PostgresParser) getTableName(query string) (tableName string, err error) {
	return getSqlTableName(query)
}

func (p *PostgresParser) setEvent(ctx context.Context, s store, auditTableName string, event Event, tableName, query string, args []interface{}) (Event, error) {
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

func (p *PostgresParser) getOldValues(ctx context.Context, db store, ww WhereClause, sqlAction string, query string, args []interface{}) (output string, w WhereClause, err error) {
	oldValues, w, err := p.runQuery(ctx, db, ww, sqlAction, query, args)
	if err != nil {
		return "", WhereClause{}, err
	}

	return string(oldValues), w, nil
}

type WhereStmt struct {
	Version int `json:"version"`
	Stmts   []struct {
		Stmt struct {
			DeleteStmt struct {
				Relation struct {
					Relname        string `json:"relname"`
					Inh            bool   `json:"inh"`
					Relpersistence string `json:"relpersistence"`
					Location       int    `json:"location"`
				} `json:"relation"`
				WhereClause struct {
					AExpr struct {
						Kind string `json:"kind"`
						Name []struct {
							String struct {
								Str string `json:"str"`
							} `json:"String"`
						} `json:"name"`
						Lexpr struct {
							ColumnRef struct {
								Fields []struct {
									String struct {
										Str string `json:"str"`
									} `json:"String"`
								} `json:"fields"`
								Location int `json:"location"`
							} `json:"ColumnRef"`
						} `json:"lexpr"`
						Rexpr struct {
							ParamRef struct {
								Number   int `json:"number"`
								Location int `json:"location"`
							} `json:"ParamRef"`
						} `json:"rexpr"`
						Location int `json:"location"`
					} `json:"A_Expr"`
				} `json:"whereClause"`
			} `json:"DeleteStmt"`
			UpdateStmt struct {
				Relation struct {
					Relname        string `json:"relname"`
					Inh            bool   `json:"inh"`
					Relpersistence string `json:"relpersistence"`
					Location       int    `json:"location"`
				} `json:"relation"`
				TargetList []struct {
					ResTarget struct {
						Name string `json:"name"`
						Val  struct {
							ParamRef struct {
								Number   int `json:"number"`
								Location int `json:"location"`
							} `json:"ParamRef"`
						} `json:"val"`
						Location int `json:"location"`
					} `json:"ResTarget"`
				} `json:"targetList"`
				WhereClause struct {
					AExpr struct {
						Kind string `json:"kind"`
						Name []struct {
							String struct {
								Str string `json:"str"`
							} `json:"String"`
						} `json:"name"`
						Lexpr struct {
							ColumnRef struct {
								Fields []struct {
									String struct {
										Str string `json:"str"`
									} `json:"String"`
								} `json:"fields"`
								Location int `json:"location"`
							} `json:"ColumnRef"`
						} `json:"lexpr"`
						Rexpr struct {
							ParamRef struct {
								Number   int `json:"number"`
								Location int `json:"location"`
							} `json:"ParamRef"`
						} `json:"rexpr"`
						Location int `json:"location"`
					} `json:"A_Expr"`
				} `json:"whereClause"`
			} `json:"UpdateStmt"`
		} `json:"stmt"`
	} `json:"stmts"`
}

func (p *PostgresParser) runQuery(ctx context.Context, s store, ww WhereClause, sqlAction, query string, args []interface{}) (out []byte, w WhereClause, err error) {
	// todo: support 'IN' operator
	// todo: support key-value store

	switch sqlAction {
	case string(Update):
		j, err := pg_query.ParseToJSON(query)
		if err != nil {
			return nil, WhereClause{}, err
		}
		p.parsed = j

		jsonWhereStmt := WhereStmt{}
		err = json.Unmarshal([]byte(j), &jsonWhereStmt)
		if err != nil {
			return nil, WhereClause{}, err
		}

		whereClause := jsonWhereStmt.Stmts[0].Stmt.UpdateStmt.WhereClause
		ww.operator = whereClause.AExpr.Name[0].String.Str
		ww.col = whereClause.AExpr.Lexpr.ColumnRef.Fields[0].String.Str
		position := whereClause.AExpr.Rexpr.ParamRef.Number
		ww.val = args[position-1]

	case string(Delete):
		j, err := pg_query.ParseToJSON(query)
		if err != nil {
			return nil, WhereClause{}, err
		}

		jsonWhereStmt := WhereStmt{}
		err = json.Unmarshal([]byte(j), &jsonWhereStmt)
		if err != nil {
			return nil, WhereClause{}, err
		}

		whereClause := jsonWhereStmt.Stmts[0].Stmt.DeleteStmt.WhereClause
		ww.operator = whereClause.AExpr.Name[0].String.Str
		ww.col = whereClause.AExpr.Lexpr.ColumnRef.Fields[0].String.Str
		position := whereClause.AExpr.Rexpr.ParamRef.Number
		ww.val = args[position-1]
	default:
		return []byte("{}"), ww, nil
	}

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

func (p *PostgresParser) queryMarshal(ctx context.Context, s store, ww WhereClause) ([]byte, uint64, error) {
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

func (p *PostgresParser) Save(ctx context.Context, query string, args []interface{}, lastInsertID int64, event Event) error {
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

func (p *PostgresParser) setNewInsertValues(ctx context.Context, event Event, lastInsertID int64, query string, args []interface{}) Event {
	marshalled, err := p.marshalInsertQuery(query, lastInsertID, args)
	if err != nil {
		return Event{}
	}
	event.TableRowID = uint64(lastInsertID)
	event.NewValues = string(marshalled)
	event.CreatedAt = time.Now()

	return event
}

func (p *PostgresParser) marshalInsertQuery(query string, lastInsertID int64, args []interface{}) ([]byte, error) {
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

func (p *PostgresParser) setNewUpdateValues(ctx context.Context, event Event, query string, args []interface{}) Event {
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

func (p *PostgresParser) marshallFromUpdateQueryArgs(query string, args []interface{}) ([]byte, error) {
	jsonWhereStmt := WhereStmt{}
	err := json.Unmarshal([]byte(p.parsed), &jsonWhereStmt)
	if err != nil {
		return nil, err
	}

	targetList := jsonWhereStmt.Stmts[0].Stmt.UpdateStmt.TargetList
	toString := make(map[string]string, len(targetList)+1)

	for _, col := range targetList {
		colName := col.ResTarget.Name
		pos := col.ResTarget.Val.ParamRef.Number

		toString[colName] = args[pos-1].(string)
	}

	marshalled, err := json.Marshal(toString)
	if err != nil {
		return nil, err
	}

	return marshalled, nil
}
