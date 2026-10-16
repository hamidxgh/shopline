package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
)

const productsBucket = "products"

func newObjectStore() (*minio.Client, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	access := os.Getenv("S3_ACCESS_KEY")
	if access == "" {
		access = "shopline"
	}
	secret := os.Getenv("S3_SECRET_KEY")
	if secret == "" {
		secret = "shopline-secret"
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: false, // lab HTTP, not HTTPS
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	return client, nil
}

func uploadProductImage(db *pgxpool.Pool, objects *minio.Client, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		const maxUpload = 10 << 20 // 10 MiB
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			http.Error(w, "missing form field image", http.StatusBadRequest)
			return
		}
		defer file.Close()

		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".bin"
		}
		key := strconv.FormatInt(id, 10) + ext
		ctx := r.Context()

		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		_, err = objects.PutObject(ctx, productsBucket, key, file, header.Size, minio.PutObjectOptions{
			ContentType: contentType,
		})
		if err != nil {
			log.Printf("s3 put %s: %v", key, err)
			http.Error(w, "upload failed", http.StatusBadGateway)
			return
		}

		tag, err := db.Exec(ctx,
			`UPDATE products SET image_key = $1 WHERE id = $2`, key, id)
		if err != nil {
			log.Printf("image_key update: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if tag.RowsAffected() == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		cacheKey := "product:" + strconv.FormatInt(id, 10)
		if err := rdb.Del(ctx, cacheKey).Err(); err != nil {
			log.Printf("cache del %s: %v", cacheKey, err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"image_key": key})
	}
}
