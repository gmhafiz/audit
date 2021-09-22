package audit

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

func (a *Auditor) SetEvent(ctx context.Context, event Event, tableName, query string, args []interface{}) (Event, error) {
	switch a.parser.parserType {
	case MysqlDB:
		return a.store.parser.MysqlParser.setEvent(ctx, a.store, a.auditTableName, event, tableName, query, args)
	case PostgresDB:
		return a.store.parser.PostgresParser.setEvent(ctx, a.store, a.auditTableName, event, tableName, query, args)
	default:
		return Event{}, ErrDriverNotSupported
	}
}

type WhereClause struct {
	id        uint64
	tableName string
	col       string
	operator  string
	val       interface{}
}

func (a *Auditor) Save(ctx context.Context, query string, args []interface{}, lastInsertID int64, event Event) error {
	var err error
	switch a.dbType {
	case MysqlDB:
		err = a.parser.MysqlParser.Save(ctx, query, args, lastInsertID, event)
	case PostgresDB:
		err = a.parser.PostgresParser.Save(ctx, query, args, lastInsertID, event)
	default:
		return ErrDriverNotSupported
	}

	return err
}

func getColumnNamesFromInsert(query string) []string {
	r := regexp.MustCompile("into(.*)values")
	return getColumnNames(r, query)
}

func getColumnNamesFromUpdate(query string) []string {
	r := regexp.MustCompile("set(.*)where")

	idx := r.FindStringIndex(strings.ToLower(query))
	part := query[idx[0]:idx[1]]

	parts := strings.Split(part, " ")
	cols := strings.Split(parts[1], ",")

	var ret []string
	for _, col := range cols {
		splits := strings.Split(col, "=")
		ret = append(ret, splits[0])
	}
	return ret
}

func getColumnNames(r *regexp.Regexp, query string) []string {
	idx := r.FindStringIndex(strings.ToLower(query))
	part := query[idx[0]:idx[1]]

	splits := strings.Split(part, " ")
	getCols := splits[2 : len(splits)-1]
	cols := strings.Join(getCols, " ")

	cols = strings.ReplaceAll(cols, "(", "")
	cols = strings.ReplaceAll(cols, ")", "")
	cols = strings.ReplaceAll(cols, "`", "")
	splits = strings.Split(cols, ",")

	var names []string
	for _, val := range splits {
		names = append(names, strings.TrimSpace(val))
	}

	return names
}

func getNameAndWherePositionForUpdate(s string) (name string, position int, err error) {
	index := regexp.MustCompile("where(.*?)").FindStringIndex(s)
	where := strings.TrimSpace(s[index[1]:]) //  id = :v3
	splits := strings.Split(where, " ")

	position, err = strconv.Atoi(splits[2][2:])
	if err != nil {
		return "", 0, err
	}

	return splits[0], position, nil
}
