package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meilisearch/meilisearch-go"
	"github.com/redis/go-redis/v9"
)

type Product struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	PriceCents int64   `json:"price_cents"`
	Stock      int     `json:"stock"`
	ImageKey   *string `json:"image_key,omitempty"`
}

func createProduct(db *pgxpool.Pool, search meilisearch.ServiceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p Product
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		err := db.QueryRow(r.Context(),
			`INSERT INTO products (name, price_cents, stock)
			 VALUES ($1, $2, $3) RETURNING id`,
			p.Name, p.PriceCents, p.Stock,
		).Scan(&p.ID)
		if err != nil {
			log.Printf("create product: %v", err)
			http.Error(w, "invalid product", http.StatusBadRequest)
			return
		}

		indexProduct(search, p)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	}
}


func getProduct(db *pgxpool.Pool, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		key := "product:" + strconv.FormatInt(id, 10)

		cached, err := rdb.Get(r.Context(), key).Bytes()
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.Write(cached)
			return
		}
		if !errors.Is(err, redis.Nil) {
			log.Printf("cache get %s: %v", key, err)
		}

		var p Product
		err = db.QueryRow(r.Context(),
			`SELECT id, name, price_cents, stock, image_key FROM products WHERE id = $1`, id,
		).Scan(&p.ID, &p.Name, &p.PriceCents, &p.Stock, &p.ImageKey)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("get product: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		body, _ := json.Marshal(p)
		if err := rdb.Set(r.Context(), key, body, 5*time.Minute).Err(); err != nil {
			log.Printf("cache set %s: %v", key, err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "MISS")
		w.Write(body)
	}
}

func updatePrice(db *pgxpool.Pool, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		var in struct {
			PriceCents int64 `json:"price_cents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		tag, err := db.Exec(r.Context(),
			`UPDATE products SET price_cents = $1 WHERE id = $2`, in.PriceCents, id)
		if err != nil {
			log.Printf("update price: %v", err)
			http.Error(w, "invalid price", http.StatusBadRequest)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		key := "product:" + strconv.FormatInt(id, 10)
		if err := rdb.Del(r.Context(), key).Err(); err != nil {
			log.Printf("cache del %s: %v", key, err)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}