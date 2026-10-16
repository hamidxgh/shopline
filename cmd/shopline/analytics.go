package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func clickhouseDSN() string {
	if v := os.Getenv("CLICKHOUSE_URL"); v != "" {
		return v
	}
	return "http://localhost:8123"
}

func recordOrderEvent(orderID, productID, cents int64) {
	q := url.Values{}
	q.Set("user", envOr("CLICKHOUSE_USER", "shopline"))
	q.Set("password", envOr("CLICKHOUSE_PASSWORD", "shopline"))
	q.Set("database", envOr("CLICKHOUSE_DB", "shopline"))
	q.Set("query", fmt.Sprintf(
		"INSERT INTO order_events (order_id, product_id, cents) VALUES (%d, %d, %d)",
		orderID, productID, cents,
	))

	resp, err := http.Post(clickhouseDSN()+"/?"+q.Encode(), "text/plain", strings.NewReader(""))
	if err != nil {
		log.Printf("clickhouse insert: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("clickhouse insert status %d", resp.StatusCode)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}