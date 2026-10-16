package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
)

func startOutboxRelay(ctx context.Context, db *pgxpool.Pool, w *kafka.Writer) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("outbox relay stopped")
				return
			case <-ticker.C:
				n, err := relayOnce(context.Background(), db, w)
				if err != nil {
					log.Printf("outbox relay: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("outbox relay published %d", n)
				}
			}
		}
	}()
}

func relayOnce(ctx context.Context, db *pgxpool.Pool, w *kafka.Writer) (int, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, topic, payload
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT 100
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []int64
	var msgs []kafka.Message
	for rows.Next() {
		var id int64
		var topic string
		var payload []byte
		if err := rows.Scan(&id, &topic, &payload); err != nil {
			return 0, err
		}
		ids = append(ids, id)
		msgs = append(msgs, kafka.Message{Topic: topic, Value: payload})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(msgs) == 0 {
		return 0, nil
	}

	if err := w.WriteMessages(ctx, msgs...); err != nil {
		return 0, err
	}

	_, err = tx.Exec(ctx,
		`UPDATE outbox SET published_at = now() WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(msgs), nil
}
