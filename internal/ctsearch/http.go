package ctsearch

import (
	"net/http"
	"time"
)

// httpClientBuilder returns an *http.Client tuned for synchronous, one-at-a-time
// CT lookups: no excessive connection pools, a bounded idle lifetime, and a
// per-request timeout from the supplied Config.
func httpClientBuilder(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultConfig().Timeout
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        4,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  false,
		},
	}
}
