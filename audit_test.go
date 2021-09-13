package audit

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

type repository struct {
	db *sqlx.DB
}

func NewMock() (*sqlx.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		log.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	return sqlxDB, mock
}

func New(db *sqlx.DB) *repository {
	return &repository{db: db}
}

func TestNewAudit(t *testing.T) {
	db, mock := NewMock()
	repo := New(db)
	t.Log(mock)
	t.Log(repo)

	auditor, err := NewAudit(db.DB, "mysql")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(auditor)


}

func TestSetAudit(t *testing.T) {
	db, mock := NewMock()
	repo := New(db)
	t.Log(mock)
	t.Log(repo)

	auditor, err := NewAudit(db.DB, "mysql")
	if err != nil {
		t.Fatal(err)
	}

	createdAt := time.Now()

	auditor.SetEvent(&Event{
		Organisation: 1,
		ActorID:      1,
		Table:        "users",
		Action:       Create,
		OldValues:    "{}",
		NewValues:    "{}",
		HTTPMethod:   "post",
		URL:          "https://example.com/api/users",
		IPAddress:    "127.0.0.1",
		UserAgent:    "Mozilla/",
		CreatedAt:    createdAt,
	})
}


func TestSaveAudit(t *testing.T) {
	db, mock := NewMock()
	repo := New(db)
	t.Log(mock)
	t.Log(repo)

	auditor, err := NewAudit(db.DB, "mysql")
	if err != nil {
		t.Fatal(err)
	}

	createdAt := time.Now()

	auditor.SetEvent(&Event{
		Organisation: 1,
		ActorID:      1,
		Table:        "users",
		Action:       Create,
		OldValues:    "{}",
		NewValues:    "{}",
		HTTPMethod:   "post",
		URL:          "https://example.com/api/users",
		IPAddress:    "127.0.0.1",
		UserAgent:    "Mozilla/",
		CreatedAt:    createdAt,
	})

	err = auditor.Save(context.Background())
	if err != nil {
		t.Error(err)
	}
}