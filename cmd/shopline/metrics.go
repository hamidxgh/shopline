package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by method, path, and status.",
	}, []string{"method", "path", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and path.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
)

func metricsHandler() http.Handler {
	return promhttp.Handler()
}

func withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		path := r.Pattern
		if path == "" {
			path = r.URL.Path
		}
		labels := prometheus.Labels{
			"method": r.Method,
			"path":   path,
			"status": strconv.Itoa(ww.status),
		}
		httpRequests.With(labels).Inc()
		httpDuration.With(prometheus.Labels{
			"method": r.Method,
			"path":   path,
		}).Observe(time.Since(start).Seconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
