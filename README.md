# Introduction

Audit database queries and execs by making use of interceptors from [ngrok/sqlmw](github.com/ngrok/sqlmw).

Any record modifications including `insert`, `update` and `delete` create a new record in the `audits` table. Audit values like 
  - who is making the change
  - old value
  - new value 
  - modification time

among others are recorded.

Full list:
```go
type Event struct {
    Organisation uint64    `db:"organisation"` // or tenant
    ActorID      uint64    `db:"actor_id"`
    TableRowID   uint64    `db:"table_row_id"`
    Table        string    `db:"table_name"`
    Action       Action    `db:"action"`
    OldValues    string    `db:"old_values"`
    NewValues    string    `db:"new_values"`
    HTTPMethod   string    `db:"http_method"`
    URL          string    `db:"url"`
    IPAddress    string    `db:"ip_address"`
    UserAgent    string    `db:"user_agent"`
    CreatedAt    time.Time `db:"created_at"`
}
```

Both `old_values` and `new_values` are stored in JSON format. For example:

| id | organisation\_id | actor\_id | table\_row\_id | table\_name | action | old\_values | new\_values | http\_method | url | ip\_address | user\_agent | created\_at |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| 42 | 1 | 2 | 15 | users | update | {"name":"test name","id":"42"} | {"name":"changed name","id":"42"} | PUT | /api/v1/user/42 | localhost:8080 | PostmanRuntime/7.28.4 | 2021-09-15 02:10:02 |


# Install

    go get github.com/gmhafiz/audit
    go get github.com/go-sql-driver/mysql

# Usage

1. Open database connection pool and register the database middleware and auditor

Create a new audit instance
```go
auditor, err := audit.NewAudit()
```
Optionally, you can customize the audit table name
```go
auditor, err := audit.NewAudit(audit.WithTableName("other_audit_table_name")) // only alphanumeric name is accepted 
```

You can also add a list of tables to be exempted:
```go
auditor, err := audit.NewAudit(
    audit.WithTableName("other_audit_table_name"),
    audit.WithTableException("schema_migrations", "other_tables"),
)
```
Add the code to where you open database connection:
```go
package database

import (
    "database/sql"
    "log"
    
    "github.com/gmhafiz/audit"
    "github.com/go-sql-driver/mysql"
    "github.com/ngrok/sqlmw"
)

func NewDB(dataSourceName string) (*sql.DB, auditor *audit.Auditor) {
    // initialise auditor
    auditor, err := audit.NewAudit()
    if err != nil {
        log.Fatal(err)
    }
   
    // register sql interceptor and our custom driver
    databaseDriverName, err := audit.RegisterDriverInterceptor(auditor, "mysql")
    if err != nil {
        log.Fatal(err)
    }
    
    // open database connection using that driver
    db, err := sql.Open(databaseDriverName, dataSourceName)
    if err != nil {
        log.Fatal(err)
    }
```
Adding your application database instance is compulsory and is done after connection pool is opened. This is used to query and save both old values and new values of the affected record.
```go
    err = auditor.SetDB(
        audit.Sql(db),
    )
    if err != nil {
        log.Fatal(err)
    }
   
    return db, auditor
}
```

By setting the database, the library will create an `audits` table automatically for you. Index creation is left to the user. Often, you would want a composite index of (`table_row_id`, `table_name`) - and possibly a separate `actor_id` index.

For Mysql:
```sql
create index audits_table_row_id_table_name_index
	on audits (table_row_id, table_name);
create index audits_actor_id_index
    on audits (actor_id);
```

2. A middleware is needed to capture current user ID and optionally organisation/tenant ID from the current request context. In order to use it, both user ID and organisation ID must be saved into `context` in `UserID` and `OrganisationID` respectively. These two values are retrieved from JWT or session cookies.

```go
import (
	"github.com/gmhafiz/audit/middleware"
)

func Auth(store *redisstore.RedisStore) Adapter {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
            var organisationID = get session.Values["organisationID"]
            var userID = session.Values["userID"]
            
            ctx := context.WithValue(r.Context(), middleware.OrganisationID, organisationID)
            ctx = context.WithValue(ctx, middleware.UserID, userID)
            
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

3. Use the provided middleware that captures current user ID and optionally organisation ID by registering it to your router.

Your own `Auth` middleware has to be registered before `middleware.Audit`.

```go
import (
    "github.com/gmhafiz/audit/middleware"
    "github.com/go-chi/chi/v5"
)

func router() *chi.Mux {
    r := chi.NewRouter()
    r.Use(middleware.Auth)
    r.Use(middleware.Audit)
   
    return r
}
```


# Test

1. Create an appropriate testing database for each postgres and mysql
2. Set both `MYSQL_DSN` and `POSTGRES_DSN` in your environment variable.


    POSTGRES_DSN=host=0.0.0.0 port=5432d user=users password=password dbname=audit_test sslmode=disable
    MYSQL_DSN=root:password@tcp(0.0.0.0:3306)/audit_test?parseTime=true&interpolateParams=true


3. Run


    go test ./...

# Limitations

1. [Table ID](#table-id)
2. [Hooks](#hooks)
3. [Login](#login)
4. [`IN` operator](#IN-operator)

## Table ID

This library assumes your table ids are in `uint64` format and named `id`,

## Hooks


## Login

Scenario: Say for every login, you save the time that user last logged in - and you want this to be audited.

Remember that an `Auth` middleware is needed to capture user id (from JWT or session token). This audit library won't be able to capture the IDs because you do not place an `Auth` middleware to logging in a user.

To work around this, save the IDs manually before making a call to set user's last login time.
```go
func (u *userService) Login(ctx context.Context, loginReq *LoginRequest) (*models.User, error) {
    user, err := u.repository.Get(ctx, loginReq)
    if err != nil {
        return nil, err
    }
    isValidPassword, err := checkValidPassword(loginReq.Password, user.Password)
    if err != nil {
        return nil, err
    }
	
	// once you've checked that the login is valid, you may save the IDs
    ctx = context.WithValue(ctx, "userID", user.ID)

    // Finally, you may set the time this user last login. The hooks will be applied since you are making an `update` database operation.
    err = u.repository.SetLastLogin(ctx, user)

    return user, nil
}
```

## IN Operator

Currently, this audit package doesn't parse `IN` operator correctly. 

# Credit

 - https://github.com/qustavo/sqlhooks/ for the SQL hooks.
 - https://github.com/owen-it/laravel-auditing for the motivation (no similar library has existed for Go) and schema inspiration.