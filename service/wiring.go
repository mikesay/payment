package service

import (
	"net/http"
	"os"

	"context"

	"github.com/go-kit/log"

	stdopentracing "github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/weaveworks/common/middleware"
)

var (
	HTTPLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Time (in seconds) spent serving HTTP requests.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status_code", "isWS"})

	HTTPRequestActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http_request_active",
		Help: "The number of HTTP requests currently being handled.",
	}, []string{"method", "path"})

	HTTPRequestSizeBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_size_bytes",
		Help: "Size of HTTP request bodies in bytes.",
		// Exponential buckets are better for sizes (e.g., 100B to 10MB).
		Buckets: prometheus.ExponentialBuckets(100, 10, 6),
	}, []string{"method", "handler"})

	HTTPResponseSizeBytes = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_size_bytes",
		Help:    "Size of HTTP response bodies in bytes.",
		Buckets: prometheus.ExponentialBuckets(100, 10, 6),
	}, []string{"method", "handler"})
)

func init() {
	prometheus.MustRegister(HTTPLatency)
	prometheus.MustRegister(HTTPRequestActive)
	prometheus.MustRegister(HTTPRequestSizeBytes)
	prometheus.MustRegister(HTTPResponseSizeBytes)
}

func WireUp(ctx context.Context, declineAmount float32, tracer stdopentracing.Tracer, serviceName string) (http.Handler, log.Logger) {
	// Log domain.
	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = log.With(logger, "ts", log.DefaultTimestampUTC)
		logger = log.With(logger, "caller", log.DefaultCaller)
	}

	// Service domain.
	var service Service
	{
		service = NewAuthorisationService(declineAmount)
		service = LoggingMiddleware(logger)(service)
	}

	// Endpoint domain.
	endpoints := MakeEndpoints(service, tracer)

	router := MakeHTTPHandler(ctx, endpoints, logger, tracer)

	httpMiddleware := []middleware.Interface{
		middleware.Instrument{
			Duration:         HTTPLatency,
			RouteMatcher:     router,
			InflightRequests: HTTPRequestActive,
			RequestBodySize:  HTTPRequestSizeBytes,
			ResponseBodySize: HTTPResponseSizeBytes,
		},
	}

	// Handler
	handler := middleware.Merge(httpMiddleware...).Wrap(router)

	return handler, logger
}
