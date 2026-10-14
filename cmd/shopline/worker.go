package main

import (
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func startEmailWorker(ch *amqp.Channel) error {
	if err := ch.Qos(1, 0, false); err != nil {
		return err
	}

	deliveries, err := ch.Consume(
		emailQueue,
		"",    // consumer tag: broker assigns one
		false, // autoAck: we ack only after "sending"
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for d := range deliveries {
			var task emailTask
			if err := json.Unmarshal(d.Body, &task); err != nil {
				log.Printf("email bad payload: %v", err)
				_ = d.Nack(false, false) // drop poison message
				continue
			}
			// stand-in for SMTP / SendGrid / SES
			log.Printf("email sent order=%d total_cents=%d", task.OrderID, task.TotalCents)
			if err := d.Ack(false); err != nil {
				log.Printf("email ack: %v", err)
			}
		}
	}()

	return nil
}
