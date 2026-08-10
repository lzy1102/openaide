package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// newPGVectorMock builds a PGVector wired to a sqlmock DB + scripted embedder.
func newPGVectorMock(t *testing.T, embedResp string) (*PGVector, sqlmock.Sqlmock, *httptest.Server) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var embSrv *httptest.Server
	if embedResp != "" {
		embSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, embedResp)
		}))
		t.Cleanup(embSrv.Close)
	}
	pv := &PGVector{
		db:         db,
		emb:        newEmbedder("http://localhost:1", "", "m"),
		collection: "openaide_docs",
	}
	if embSrv != nil {
		pv.emb = newEmbedder(embSrv.URL, "", "m")
	}
	return pv, mock, embSrv
}

func TestPGVector_Migrate(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, "")
	mock.ExpectExec("CREATE EXTENSION IF NOT EXISTS vector").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS openaide_docs")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("CREATE INDEX IF NOT EXISTS openaide_docs_vec_idx")).WillReturnResult(sqlmock.NewResult(0, 0))

	if err := pv.migrate(context.Background()); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPGVector_Migrate_Error(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, "")
	mock.ExpectExec("CREATE EXTENSION IF NOT EXISTS vector").WillReturnError(fmt.Errorf("no permission"))
	if err := pv.migrate(context.Background()); err == nil {
		t.Fatal("expected migrate error")
	}
}

func TestPGVector_Index_Success(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, `{"data":[{"embedding":[0.1,0.2]},{"embedding":[0.3,0.4]}]}`)
	// Embedding endpoint hit, then a transaction with 2 upserts.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO openaide_docs")).
		WithArgs("id1", "content1", "[0.1,0.2]", "{}").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO openaide_docs")).
		WithArgs("id2", "content2", "[0.3,0.4]", "{}").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err := pv.Index(context.Background(), "openaide_docs", []Document{
		{ID: "id1", Content: "content1", Metadata: map[string]string{}},
		{ID: "id2", Content: "content2", Metadata: map[string]string{}},
	})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPGVector_Index_BeginError(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, `{"data":[{"embedding":[0.1]}]}`)
	mock.ExpectBegin().WillReturnError(fmt.Errorf("tx busy"))
	err := pv.Index(context.Background(), "c", []Document{{ID: "1", Content: "x"}})
	if err == nil {
		t.Fatal("expected tx error")
	}
}

func TestPGVector_Search_Success(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, `{"data":[{"embedding":[0.1,0.2]}]}`)
	rows := sqlmock.NewRows([]string{"id", "content", "metadata", "score"}).
		AddRow("id1", "content1", `{"path":"a.go"}`, 0.95)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, content, metadata, 1 - (embedding <=> $1) AS score")).
		WithArgs("[0.1,0.2]", 5).
		WillReturnRows(rows)

	results, err := pv.Search(context.Background(), "openaide_docs", "query", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "id1" || results[0].Score != 0.95 {
		t.Errorf("unexpected result: %+v", results[0])
	}
	if results[0].Metadata["path"] != "a.go" {
		t.Errorf("expected metadata parsed, got %+v", results[0].Metadata)
	}
}

func TestPGVector_Delete_Success(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, "")
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM openaide_docs WHERE id IN ($1,$2)")).
		WithArgs("a", "b").
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := pv.Delete(context.Background(), "openaide_docs", []string{"a", "b"}); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPGVector_Ping(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, "")
	mock.ExpectPing()
	if err := pv.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestPGVector_Ping_Error(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, "")
	mock.ExpectPing().WillReturnError(fmt.Errorf("db down"))
	if err := pv.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error")
	}
}

// TestPGVector_Index_Metadata ensures metadata JSON survives round-trip.
func TestPGVector_Index_Metadata(t *testing.T) {
	pv, mock, _ := newPGVectorMock(t, `{"data":[{"embedding":[0.5]}]}`)
	meta := map[string]string{"path": "a.go", "lang": "go"}
	metaJSON, _ := json.Marshal(meta)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO openaide_docs")).
		WithArgs("id1", "content", "[0.5]", string(metaJSON)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := pv.Index(context.Background(), "openaide_docs", []Document{{ID: "id1", Content: "content", Metadata: meta}})
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
