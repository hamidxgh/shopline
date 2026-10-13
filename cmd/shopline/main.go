package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

//go:embed schema.sql
var schemaSQL string

func main() {
	ctx := context.Background()

	shutdown, err := initTracer(ctx)
	if err != nil {
		log.Fatalf("tracer: %v", err)
	}
	defer func() { _ = shutdown(context.Background()) }()

	auth, err := newAuthenticator(ctx)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}
	log.Println("auth: keycloak verifier ready")

	if _, err := readVaultDBPassword(); err != nil {
		log.Fatalf("vault: %v", err)
	}
	log.Println("vault: loaded secret/shopline")

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://shopline:shopline@localhost:5432/shopline"
	}

	// postgres
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("db config: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("db ping: %v", err)
	}
	log.Println("connected to postgres")

	if _, err := db.Exec(ctx, schemaSQL); err != nil {
		log.Fatalf("db exec: %v", err)
	}
	log.Println("schema applied")

	replicaDSN := os.Getenv("DATABASE_REPLICA_URL")
	if replicaDSN == "" {
		replicaDSN = dsn // local fallback: same as primary
	}
	dbRead, err := pgxpool.New(ctx, replicaDSN)
	if err != nil {
		log.Fatalf("db replica config: %v", err)
	}
	defer dbRead.Close()
	if err := dbRead.Ping(ctx); err != nil {
		log.Fatalf("db replica ping: %v", err)
	}
	log.Println("connected to postgres replica")

	// redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("redis unavailable, running without cache: %v", err)
	} else {
		log.Println("connected to redis")
	}

	// rabbitmq
	conn, ch, err := connectQueue()
	if err != nil {
		log.Fatalf("rabbitmq: %v", err)
	}
	defer conn.Close()
	defer ch.Close()
	log.Println("connected to rabbitmq")

	// email worker
	if err := startEmailWorker(ch); err != nil {
	log.Fatalf("email worker: %v", err)
}
	log.Println("email worker started")

	events := newEventWriter()
	defer events.Close()
	log.Println("event writer ready")

	// outbox relay: only the etcd-elected leader runs it
	runOutboxLeader(ctx, func(leadCtx context.Context) {
		startOutboxRelay(leadCtx, db, events)
		<-leadCtx.Done()
	})
	log.Println("outbox leader election started")

	// s3
	objects, err := newObjectStore()
	if err != nil {
		log.Fatalf("object store: %v", err)
	}
	log.Println("connected to object store")

	search := newSearchClient()
	log.Println("connected to meilisearch")

	// routes
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.Handle("GET /metrics", metricsHandler())

	mux.HandleFunc("POST /products", createProduct(db, search))
	mux.HandleFunc("GET /products/{id}", getProduct(dbRead, rdb))
	mux.HandleFunc("PATCH /products/{id}", updatePrice(db, rdb))
	mux.HandleFunc("POST /products/{id}/image", uploadProductImage(db, objects, rdb))
	mux.HandleFunc("GET /search", searchProducts(search))
	mux.HandleFunc("POST /orders", createOrder(db, rdb, ch))

	// start server
	host, _ := os.Hostname()
	handler := otelhttp.NewHandler(
		auth.middleware(withMetrics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Instance", host)
			mux.ServeHTTP(w, r)
		}))),
		"shopline",
	)

	log.Printf("shopline %s listening on :8080", host)
	log.Fatal(http.ListenAndServe(":8080", handler))
}