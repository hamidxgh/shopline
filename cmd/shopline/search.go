package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

const productsIndex = "products"

func newSearchClient() meilisearch.ServiceManager {
	url := os.Getenv("MEILI_URL")
	if url == "" {
		url = "http://localhost:7700"
	}
	key := os.Getenv("MEILI_MASTER_KEY")
	if key == "" {
		key = "shopline-masterkey"
	}
	return meilisearch.New(url, meilisearch.WithAPIKey(key))
}

type productDoc struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
	Stock      int    `json:"stock"`
}

func indexProduct(client meilisearch.ServiceManager, p Product) {
	doc := productDoc{
		ID:         p.ID,
		Name:       p.Name,
		PriceCents: p.PriceCents,
		Stock:      p.Stock,
	}
	task, err := client.Index(productsIndex).AddDocuments([]productDoc{doc}, nil)
	if err != nil {
		log.Printf("meili index product %d: %v", p.ID, err)
		return
	}
	if _, err := client.WaitForTask(task.TaskUID, 50*time.Millisecond); err != nil {
		log.Printf("meili wait product %d: %v", p.ID, err)
	}
}

func searchProducts(client meilisearch.ServiceManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "missing q", http.StatusBadRequest)
			return
		}
		res, err := client.Index(productsIndex).Search(q, &meilisearch.SearchRequest{
			Limit: 20,
		})
		if err != nil {
			log.Printf("meili search: %v", err)
			http.Error(w, "search failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"query": q,
			"hits":  res.Hits,
		})
	}
}