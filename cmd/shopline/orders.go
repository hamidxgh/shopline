package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	amqp "github.com/rabbitmq/amqp091-go"
)

type orderItemInput struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type orderLine struct {
	orderItemInput
	Name       string
	PriceCents int64
}

func createOrder(db *pgxpool.Pool, rdb *redis.Client, ch *amqp.Channel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Items []orderItemInput `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
			http.Error(w, "invalid order", http.StatusBadRequest)
			return
		}
		for _, it := range req.Items {
			if it.Quantity <= 0 {
				http.Error(w, "quantity must be positive", http.StatusBadRequest)
				return
			}
		}
		slices.SortFunc(req.Items, func(a, b orderItemInput) int {
			return cmp.Compare(a.ProductID, b.ProductID)
		})

		ctx := r.Context()
		tx, err := db.Begin(ctx)
		if err != nil {
			log.Printf("order begin: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(ctx)

		var lines []orderLine
		var total int64
		for _, it := range req.Items {
			l := orderLine{orderItemInput: it}
			err := tx.QueryRow(ctx,
				`UPDATE products SET stock = stock - $1
				 WHERE id = $2 AND stock >= $1
				 RETURNING name, price_cents`,
				it.Quantity, it.ProductID,
			).Scan(&l.Name, &l.PriceCents)
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "out of stock or unknown product", http.StatusConflict)
				return
			}
			if err != nil {
				log.Printf("order reserve: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			lines = append(lines, l)
			total += l.PriceCents * int64(l.Quantity)
		}

		var orderID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO orders (total_cents) VALUES ($1) RETURNING id`, total,
		).Scan(&orderID); err != nil {
			log.Printf("order insert: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		for _, l := range lines {
			if _, err := tx.Exec(ctx,
				`INSERT INTO order_items (order_id, product_id, product_name, unit_price_cents, quantity)
				 VALUES ($1, $2, $3, $4, $5)`,
				orderID, l.ProductID, l.Name, l.PriceCents, l.Quantity,
			); err != nil {
				log.Printf("order item: %v", err)
				http.Error(w, "invalid order", http.StatusBadRequest)
				return
			}
		}

		payload, err := json.Marshal(orderPlacedEvent{OrderID: orderID, TotalCents: total})
		if err != nil {
			log.Printf("outbox marshal: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO outbox (topic, payload) VALUES ($1, $2)`,
			orderPlacedTopic, payload,
		); err != nil {
			log.Printf("outbox insert: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(ctx); err != nil {
			log.Printf("order commit: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		keys := make([]string, len(lines))
		for i, l := range lines {
			keys[i] = "product:" + strconv.FormatInt(l.ProductID, 10)
		}
		if err := rdb.Del(ctx, keys...).Err(); err != nil {
			log.Printf("cache del %v: %v", keys, err)
		}

		if err := publishEmail(ctx, ch, orderID, total); err != nil {
			log.Printf("email publish order %d: %v", orderID, err)
		}

		for _, l := range lines {
			recordOrderEvent(orderID, l.ProductID, l.PriceCents*int64(l.Quantity))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"order_id": orderID, "total_cents": total})
	}
}
