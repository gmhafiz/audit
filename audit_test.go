package audit

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	//pg_query "github.com/pganalyze/pg_query_go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	CreateTablePostgres = `CREATE TABLE IF NOT EXISTS users (id bigserial primary key, email text null);`
	CreateTableMysql    = `CREATE TABLE IF NOT EXISTS users( id bigint unsigned auto_increment primary key, email text null);`
)

type suite struct {
	db      *sql.DB
	auditor *Auditor
}

func TestPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", "0.0.0.0", 5432, "user", "password", "audit_test")
		//t.Skipf("POSTGRES_DSN not set")
	}

	//j, err := pg_query.ParseToJSON("INSERT INTO users (email) VALUES ($1)")
	//assert.NoError(t, err)
	//fmt.Println(j)
	//
	//result, err := pg_query.Parse("INSERT INTO users (email) VALUES ($1)")
	//if err != nil {
	//	panic(err)
	//}
	//columns := result.Stmts[0].Stmt.GetInsertStmt().GetCols()
	//fmt.Println(result)
	//
	//for _, col := range columns {
	//	name := col.GetResTarget().Name
	//	val := col.GetResTarget().Val
	//	fmt.Println(name)
	//	fmt.Println(val)
	//}
	//tableName := result.Stmts[0].Stmt.GetInsertStmt().GetRelation().Relname
	//fmt.Println(tableName)

	setupTable(t, dsn, PostgresDB)

	auditTableName := "audits"
	s := newSqlSuite(t, "postgres", dsn, auditTableName)

	s.CleanUp(t, "TRUNCATE users RESTART IDENTITY;")
	s.CleanUp(t, fmt.Sprintf("TRUNCATE %s RESTART IDENTITY;", auditTableName))
	s.TestFails(t, "INSERT INTO users (email) VALUES ($1) RETURNING id", "email@example.com")
	s.TestInsertPostgres(t, "INSERT INTO users (email) VALUES ($1) RETURNING id", "email@example.com")
	s.TestUpdate(t, "UPDATE users SET email=$1 where id=$2", 1, "edited@example.com")
	s.TestDelete(t, "DELETE FROM users where id=$1", 1)
}

func TestMysql(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		dsn = "root:password@tcp(0.0.0.0:3306)/audit_test?parseTime=true&interpolateParams=true"
		//t.Skipf("MYSQL_DSN not set")
	}
	setupTable(t, dsn, MysqlDB)

	auditTableName := "audits"
	s := newSqlSuite(t, "mysql", dsn, auditTableName)

	s.CleanUp(t, "TRUNCATE users;")
	s.CleanUp(t, fmt.Sprintf("TRUNCATE %s;", auditTableName))
	s.TestFails(t, "INSERT INTO users (email) VALUES(?)", "email@example.com")
	s.TestInsert(t, "INSERT INTO users (email) VALUES(?)", "email@example.com")
	s.TestUpdate(t, "UPDATE users SET email=? where id=?", 1, "edited@example.com")
	s.TestDelete(t, "DELETE FROM users where id=?", 1)
}

func (s *suite) TestFails(t *testing.T, query string, arg0 string) {
	ctx := context.Background()

	t.Run("no audit set", func(t *testing.T) {
		_, err := s.db.ExecContext(ctx, query, arg0)
		assert.Equal(t, err, ErrNoAuditSet)
	})
}

func (s *suite) TestInsert(t *testing.T, query string, args ...interface{}) {
	ctx := context.Background()

	t.Run("insert", func(t *testing.T) {
		ctx = context.WithValue(ctx, "userID", uint64(1))
		event := Event{
			HTTPMethod: "POST",
			URL:        "https://site.test/api/user",
			IPAddress:  "127.0.0.1",
			UserAgent:  "Mozilla/5.0 (X11; Linux x86_64; rv:10.0) Gecko/20100101 Firefox/10.0",
		}

		ctx := context.WithValue(ctx, "audit", event)
		_, err := s.db.ExecContext(ctx, query, args[0])
		assert.NoError(t, err)
	})
}

func (s *suite) TestInsertPostgres(t *testing.T, query string, args ...interface{}) {
	ctx := context.Background()

	t.Run("insert", func(t *testing.T) {
		ctx = context.WithValue(ctx, "userID", uint64(1))
		event := Event{
			HTTPMethod: "POST",
			URL:        "https://site.test/api/user",
			IPAddress:  "127.0.0.1",
			UserAgent:  "Mozilla/5.0 (X11; Linux x86_64; rv:10.0) Gecko/20100101 Firefox/10.0",
		}

		ctx := context.WithValue(ctx, "audit", event)
		var id int
		err := s.db.QueryRowContext(ctx, query, args[0]).Scan(&id)
		assert.NoError(t, err)
	})
}

func (s *suite) TestUpdate(t *testing.T, query string, id int, email string) {
	ctx := context.Background()

	t.Run("update", func(t *testing.T) {
		ctx = context.WithValue(ctx, "userID", uint64(1))
		event := Event{
			HTTPMethod: "PUT",
			URL:        "https://site.test/api/user/1",
			IPAddress:  "127.0.0.1",
			UserAgent:  "Mozilla/5.0 (X11; Linux x86_64; rv:10.0) Gecko/20100101 Firefox/10.0",
		}
		ctx := context.WithValue(ctx, "audit", event)

		_, err := s.db.ExecContext(ctx, query, email, id)
		assert.NoError(t, err)
	})
}

func (s *suite) TestDelete(t *testing.T, query string, id int) {
	ctx := context.Background()
	t.Run("delete", func(t *testing.T) {
		ctx = context.WithValue(ctx, "userID", uint64(1))
		event := Event{
			HTTPMethod: "DELETE",
			URL:        "https://site.test/api/user/1",
			IPAddress:  "127.0.0.1",
			UserAgent:  "Mozilla/5.0 (X11; Linux x86_64; rv:10.0) Gecko/20100101 Firefox/10.0",
		}
		ctx := context.WithValue(ctx, "audit", event)

		_, err := s.db.ExecContext(ctx, query, id)
		assert.NoError(t, err)
	})
}

func (s *suite) CleanUp(t *testing.T, query string) {
	ctx := context.Background()
	_, err := s.auditor.store.internal.ExecContext(ctx, query)
	assert.NoError(t, err)
}

func newSqlSuite(t *testing.T, driver string, dsn string, auditTableName string) *suite {
	auditor, err := NewAudit(
		WithTableName(auditTableName),
		WithTableException("except_table"),
	)
	assert.NoError(t, err)

	_, err = RegisterHooks(auditor, "cockroachdb")
	assert.Equal(t, err, ErrInvalidDatabaseDriver)

	driverName, err := RegisterHooks(auditor, driver)
	assert.NoError(t, err)

	db, err := sql.Open(driverName, dsn)
	assert.NoError(t, err)

	suite := &suite{
		db:      db,
		auditor: auditor,
	}

	if driver == "postgres" {
		err = auditor.SetDB(
			Postgres(db, dsn),
		)
		assert.NoError(t, err)

		//suite.setInternalDB(t, "postgres", dsn) // only for testing purpose

	} else if driver == "mysql" {
		err = auditor.SetDB(
			MySql(db, dsn),
		)
		assert.NoError(t, err)

		//suite.setInternalDB(t, "mysql", dsn) // only for testing purpose
	}

	return suite
}

func setupTable(t *testing.T, dsn, dbType string) {
	switch dbType {
	case MysqlDB:
		db, err := sql.Open("mysql", dsn)
		require.NoError(t, err)
		require.NoError(t, db.Ping())
		defer db.Close()
		_, err = db.Exec(CreateTableMysql)
		require.NoError(t, err)
	case PostgresDB:
		db, err := sql.Open("postgres", dsn)
		require.NoError(t, err)
		require.NoError(t, db.Ping())
		defer db.Close()
		_, err = db.Exec(CreateTablePostgres)
		require.NoError(t, err)
	}
}

//// setInternalDB runs sql query without entering hooks.
//func (s *suite) setInternalDB(t *testing.T, dbType string, dsn string) {
//	if dbType == "mysql" {
//		db, err := sql.Open("mysql", dsn)
//		assert.NoError(t, err)
//
//		s.auditor.store.internal = db
//	} else if dbType == "postgres" {
//		db, err := sql.Open("postgres", dsn)
//		assert.NoError(t, err)
//
//		s.auditor.store.internal = db
//	} else {
//		// internal store can be different from *sql.DB.
//	}
//}
