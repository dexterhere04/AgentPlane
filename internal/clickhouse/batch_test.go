package clickhouse

import (
	"context"
	"errors"
	"testing"
)

type fakeBatch struct {
	appended int
	sent     int
	sendErr  error
}

func (b *fakeBatch) Append(...any) error { b.appended++; return nil }
func (b *fakeBatch) Send() error         { b.sent++; return b.sendErr }

func TestBatchInsertAppendsAllAndSendsOnce(t *testing.T) {
	batch := &fakeBatch{}
	rows := []int{1, 2, 3, 4}

	err := batchInsert(context.Background(),
		func(context.Context, string) (batchAppender, error) { return batch, nil },
		"INSERT ...",
		rows,
		func(b batchAppender, v *int) error { return b.Append(*v) },
	)
	if err != nil {
		t.Fatalf("batchInsert: %v", err)
	}
	if batch.appended != len(rows) {
		t.Fatalf("appended %d rows, want %d", batch.appended, len(rows))
	}
	if batch.sent != 1 {
		t.Fatalf("sent %d batches, want 1", batch.sent)
	}
}

func TestBatchInsertEmptyIsNoop(t *testing.T) {
	prepared := false
	err := batchInsert(context.Background(),
		func(context.Context, string) (batchAppender, error) {
			prepared = true
			return &fakeBatch{}, nil
		},
		"INSERT ...",
		[]int{},
		func(batchAppender, *int) error { return nil },
	)
	if err != nil {
		t.Fatalf("batchInsert: %v", err)
	}
	if prepared {
		t.Fatal("empty input should not prepare a batch")
	}
}

func TestBatchInsertPropagatesSendError(t *testing.T) {
	wantErr := errors.New("boom")
	err := batchInsert(context.Background(),
		func(context.Context, string) (batchAppender, error) {
			return &fakeBatch{sendErr: wantErr}, nil
		},
		"INSERT ...",
		[]int{1},
		func(batchAppender, *int) error { return nil },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
}

func TestTruncateTablesRejectsUnknownTable(t *testing.T) {
	c := &Client{}
	if err := c.TruncateTables(context.Background(), "traces; DROP TABLE users"); err == nil {
		t.Fatal("expected an error for an unknown/unsafe table name")
	}
}
