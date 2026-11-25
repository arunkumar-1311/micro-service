// Package proxy forwards gateway requests to downstream services, guarding
// each upstream with its own circuit breaker.
package proxy

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"logistics/pkg/httpx"
	"logistics/pkg/resilience"
)

type Upstream struct {
	name string
	rp   *httputil.ReverseProxy
	cb   *resilience.CircuitBreaker
}

func New(name, target string, log *slog.Logger) (*Upstream, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	rp.Transport = &http.Transport{
		MaxIdleConnsPerHost:   100,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}
	up := &Upstream{name: name, rp: rp, cb: resilience.NewCircuitBreaker(5, 20*time.Second)}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("upstream error", "upstream", name, "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream unavailable","code":"bad_gateway"}`))
	}
	return up, nil
}

var errUpstream5xx = errors.New("upstream 5xx")

// Handler returns a gin handler that proxies the request. rewrite maps the
// public path (/api/v1/...) to the internal one (/v1/...).
func (u *Upstream) Handler(rewrite func(string) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := u.cb.Execute(func() error {
			c.Request.URL.Path = rewrite(c.Request.URL.Path)
			rec := &statusRecorder{ResponseWriter: c.Writer}
			u.rp.ServeHTTP(rec, c.Request)
			if rec.status >= 500 {
				return errUpstream5xx
			}
			return nil
		})
		if errors.Is(err, resilience.ErrCircuitOpen) {
			httpx.Fail(c, http.StatusServiceUnavailable, "circuit_open", u.name+" is temporarily unavailable")
		}
	}
}

type statusRecorder struct {
	gin.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}
