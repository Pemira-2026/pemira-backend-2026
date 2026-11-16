package monitoring

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type requestKey struct {
	Method string
	Path   string
	Status string
}

type requestStats struct {
	Count       uint64
	Errors      uint64
	DurationSum float64
}

// Metrics is a small dependency-free Prometheus-compatible metrics collector.
// It deliberately avoids external client libraries so the backend remains easy
// to build in restricted campus environments.
type Metrics struct {
	mu       sync.Mutex
	requests map[requestKey]*requestStats
	inflight int64
	started  time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{
		requests: make(map[requestKey]*requestStats),
		started:  time.Now(),
	}
}

func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		m.mu.Lock()
		m.inflight++
		m.mu.Unlock()

		start := time.Now()
		c.Next()
		elapsed := time.Since(start).Seconds()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		key := requestKey{
			Method: c.Request.Method,
			Path:   path,
			Status: strconv.Itoa(c.Writer.Status()),
		}

		m.mu.Lock()
		m.inflight--
		st := m.requests[key]
		if st == nil {
			st = &requestStats{}
			m.requests[key] = st
		}
		st.Count++
		st.DurationSum += elapsed
		if c.Writer.Status() >= 500 {
			st.Errors++
		}
		m.mu.Unlock()
	}
}

// WritePrometheus renders metrics in the Prometheus text exposition format.
func (m *Metrics) WritePrometheus(w io.Writer) {
	m.mu.Lock()
	keys := make([]requestKey, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Path != keys[j].Path {
			return keys[i].Path < keys[j].Path
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Status < keys[j].Status
	})
	snapshot := make(map[requestKey]requestStats, len(m.requests))
	for k, v := range m.requests {
		snapshot[k] = *v
	}
	inflight := m.inflight
	m.mu.Unlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	fmt.Fprintln(w, "# HELP pemira_http_requests_total Total HTTP requests by route, method, and status.")
	fmt.Fprintln(w, "# TYPE pemira_http_requests_total counter")
	for _, k := range keys {
		st := snapshot[k]
		fmt.Fprintf(w, "pemira_http_requests_total{method=\"%s\",path=\"%s\",status=\"%s\"} %d\n",
			escape(k.Method), escape(k.Path), escape(k.Status), st.Count)
	}

	fmt.Fprintln(w, "# HELP pemira_http_request_duration_seconds HTTP request duration summary.")
	fmt.Fprintln(w, "# TYPE pemira_http_request_duration_seconds summary")
	for _, k := range keys {
		st := snapshot[k]
		labels := fmt.Sprintf("method=\"%s\",path=\"%s\",status=\"%s\"", escape(k.Method), escape(k.Path), escape(k.Status))
		fmt.Fprintf(w, "pemira_http_request_duration_seconds_sum{%s} %f\n", labels, st.DurationSum)
		fmt.Fprintf(w, "pemira_http_request_duration_seconds_count{%s} %d\n", labels, st.Count)
	}

	fmt.Fprintln(w, "# HELP pemira_http_errors_total Total 5xx responses.")
	fmt.Fprintln(w, "# TYPE pemira_http_errors_total counter")
	for _, k := range keys {
		st := snapshot[k]
		fmt.Fprintf(w, "pemira_http_errors_total{method=\"%s\",path=\"%s\",status=\"%s\"} %d\n",
			escape(k.Method), escape(k.Path), escape(k.Status), st.Errors)
	}

	fmt.Fprintln(w, "# HELP pemira_http_inflight_requests Current in-flight requests.")
	fmt.Fprintln(w, "# TYPE pemira_http_inflight_requests gauge")
	fmt.Fprintf(w, "pemira_http_inflight_requests %d\n", inflight)

	fmt.Fprintln(w, "# HELP pemira_go_goroutines Number of goroutines.")
	fmt.Fprintln(w, "# TYPE pemira_go_goroutines gauge")
	fmt.Fprintf(w, "pemira_go_goroutines %d\n", runtime.NumGoroutine())

	fmt.Fprintln(w, "# HELP pemira_go_memstats_alloc_bytes Bytes of allocated heap objects.")
	fmt.Fprintln(w, "# TYPE pemira_go_memstats_alloc_bytes gauge")
	fmt.Fprintf(w, "pemira_go_memstats_alloc_bytes %d\n", mem.Alloc)

	fmt.Fprintln(w, "# HELP pemira_process_uptime_seconds Process uptime in seconds.")
	fmt.Fprintln(w, "# TYPE pemira_process_uptime_seconds gauge")
	fmt.Fprintf(w, "pemira_process_uptime_seconds %f\n", time.Since(m.started).Seconds())
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
