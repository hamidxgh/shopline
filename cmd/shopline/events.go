package main

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
)

const orderPlacedTopic = "order.placed"

func newEventWriter() *kafka.Writer {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:19092"
	}
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers),
		Topic:        "", // per-message Topic from outbox rows
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
}

type orderPlacedEvent struct {
	OrderID    int64 `json:"order_id"`
	TotalCents int64 `json:"total_cents"`
}

func publishOrderPlaced(ctx context.Context, w *kafka.Writer, orderID, totalCents int64) error {
	body, err := json.Marshal(orderPlacedEvent{OrderID: orderID, TotalCents: totalCents})
	if err != nil {
		return err
	}
	return w.WriteMessages(ctx, kafka.Message{
		Key:   []byte(strconv.FormatInt(orderID, 10)),
		Value: body,
		Time:  time.Now().UTC(),
	})
}
