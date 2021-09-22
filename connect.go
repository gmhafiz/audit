package audit

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	MysqlCreate    = "CREATE TABLE IF NOT EXISTS %s (id bigint unsigned auto_increment primary key, actor_id bigint unsigned null, table_row_id bigint unsigned null,table_name varchar(255) null,action varchar(10) null,old_values longtext collate utf8mb4_bin null,new_values longtext collate utf8mb4_bin null,http_method varchar(11) null,url text null,ip_address text null,user_agent text null,created_at datetime null,constraint new_values    check (json_valid(new_values)),constraint old_values    check (json_valid(old_values)));"
	MysqlInsert    = "INSERT INTO %s (actor_id, table_row_id, table_name, action, old_values, new_values, http_method, url, ip_address, user_agent, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)"
	MysqlSelect    = "SELECT * FROM %s WHERE %v %s ?"
	PostgresCreate = "CREATE TABLE IF NOT EXISTS %s (id bigserial constraint audits_pk primary key, actor_id bigserial, table_row_id bigserial, table_name text, action varchar(11), old_values json, new_values json, http_method varchar(11), url text, ip_address text, user_agent text, created_at timestamp with time zone);"
	PostgresInsert = "INSERT INTO %s (actor_id, table_row_id, table_name, action, old_values, new_values, http_method, url, ip_address, user_agent, created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)"
	PostgresSelect = "SELECT * FROM %s WHERE %v %s $1" // todo: support IN operator
)

type Auditor struct {
	auditTableName string
	tableException []string

	store
}

type query struct {
	insert     string
	create     string
	selectStmt string
}

type store struct {
	dbType string
	sql    *sql.DB
	mongo  *mongo.Client

	query
	parser   *Parser
	internal *sql.DB
}

var (
	ErrInvalidQuery       = fmt.Errorf("invalid query")
	ErrDriverNotSupported = fmt.Errorf("driver is not supported")
)

const (
	MysqlDB    string = "mysql"
	PostgresDB string = "postgres"
	MongoDB    string = "mongo"
)

func (a *store) newPostgresAuditor(internal *sql.DB, db *sql.DB) error {
	_, err := db.ExecContext(context.Background(), a.query.create)
	if err != nil {
		return err
	}
	a.dbType = PostgresDB

	a.parser = NewParser(a.dbType)
	postgres := &PostgresParser{
		internal: internal,
		db:       db,
		query:    a.query,
	}
	a.parser.PostgresParser = postgres

	return err
}

func (a *store) newMysqlAuditor(internal *sql.DB, db *sql.DB) error {
	_, err := internal.ExecContext(context.Background(), a.query.create)
	if err != nil {
		return err
	}
	a.dbType = MysqlDB

	a.parser = NewParser(a.dbType)
	mysql := &MysqlParser{
		internal: internal,
		db:       db,
		query:    a.query,
	}

	a.parser.MysqlParser = mysql

	return err
}

//func (a *store) newMongoAuditor(kv *mongo.Client) error {
//	a.dbType = MongoDB
//	a.mongo = kv
//
//	return nil
//}

type DBOption func(*Auditor)

func (a *Auditor) SetDB(opts ...DBOption) error {
	for _, opt := range opts {
		opt(a)
	}

	switch a.store.dbType {
	case MysqlDB:
		fallthrough
	case PostgresDB:
		return nil
	default:
		return ErrDriverNotSupported
	}
}

func (a *Auditor) GetTableName(query string) (tableName string, err error) {
	if a.store.parser == nil {
		return "", nil
	}
	switch a.store.dbType {
	case "mysql":
		return a.parser.MysqlParser.getTableName(query)
	case "postgres":
		return a.parser.PostgresParser.getTableName(query)
	default:
		return "", ErrDriverNotSupported
	}
}

func Postgres(db *sql.DB, dsn string) DBOption {
	return func(a *Auditor) {
		internal, err := sql.Open("postgres", dsn)
		if err != nil {
			log.Fatal(err)
		}
		a.store.internal = internal

		q := query{
			insert:     fmt.Sprintf(PostgresInsert, a.auditTableName),
			create:     fmt.Sprintf(PostgresCreate, a.auditTableName),
			selectStmt: PostgresSelect,
		}
		a.store.query = q
		a.store.sql = db
		err = a.newPostgresAuditor(a.store.internal, a.store.sql)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func MySql(db *sql.DB, dsn string) DBOption {
	return func(a *Auditor) {
		internal, err := sql.Open("mysql", dsn)
		if err != nil {
			log.Fatal(err)
		}
		a.store.internal = internal

		q := query{
			insert:     fmt.Sprintf(MysqlInsert, a.auditTableName),
			create:     fmt.Sprintf(MysqlCreate, a.auditTableName),
			selectStmt: MysqlSelect,
		}
		a.store.query = q
		a.store.sql = db
		err = a.newMysqlAuditor(a.store.internal, a.store.sql)
		if err != nil {
			log.Fatal(err)
		}
	}
}
