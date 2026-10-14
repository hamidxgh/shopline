package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	emailQueue = "email"
	emailDLQ   = "email.dlq"
)

func connectQueue() (*amqp.Connection, *amqp.Channel, error) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("rabbit dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("rabbit channel: %w", err)
	}

	_, err = ch.QueueDeclare(
		emailDLQ,
		true,  // durable: queue survives broker restart
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("rabbit declare %s: %w", emailDLQ, err)
	}

	_, err = ch.QueueDeclare(
		emailQueue,
		true,  // durable: queue survives broker restart
		false, // autoDelete
		false, // exclusive
		false, // noWait
		amqp.Table{
			"x-dead-letter-exchange": "",
			"x-dead-letter-routing-key": emailDLQ,
		},
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("rabbit declare %s: %w", emailQueue, err)
	}

	return conn, ch, nil
}

type emailTask struct {
	OrderID    int64 `json:"order_id"`
	TotalCents int64 `json:"total_cents"`
}

func publishEmail(ctx context.Context, ch *amqp.Channel, orderID, totalCents int64) error {
	body, err := json.Marshal(emailTask{OrderID: orderID, TotalCents: totalCents})
	if err != nil {
		return err
	}
	return ch.PublishWithContext(ctx,
		"",         // default exchange
		emailQueue, // routing key = queue name
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // message survives broker restart
			Body:         body,
		},
	)
}