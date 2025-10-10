package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	EmittedEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sentinel_events_emitted_total",
			Help: "events successfully emitted",
		},
		[]string{"kind"},
	)
	DedupedEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sentinel_events_deduped_total",
			Help: "events dropped due to dedupe",
		},
		[]string{"kind"},
	)
	StreamErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sentinel_stream_errors_total",
			Help: "stream send errors",
		},
	)
	RedisErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sentinel_redis_errors_total",
			Help: "redis op errors",
		},
	)
	RedisReconnects = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sentinel_redis_reconnects_total",
			Help: "redis reconnect attempts",
		},
	)
)

// init pre-creates common label series at 0 so they show up immediately in Prometheus,
// even before the first event flows through.
func init() {
	for _, k := range []string{"tx", "log", "vote"} {
		EmittedEvents.WithLabelValues(k).Add(0)
		DedupedEvents.WithLabelValues(k).Add(0)
	}
}

// StartServer registers metrics to a private registry and serves /metrics on addr.
func StartServer(addr string) {
	r := prometheus.NewRegistry()
	r.MustRegister(
		EmittedEvents,
		DedupedEvents,
		StreamErrors,
		RedisErrors,
		RedisReconnects,
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(r, promhttp.HandlerOpts{}))
	go http.ListenAndServe(addr, mux)
}
