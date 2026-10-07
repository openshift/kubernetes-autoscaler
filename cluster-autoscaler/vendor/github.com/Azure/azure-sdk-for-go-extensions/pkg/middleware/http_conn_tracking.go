package middleware

import (
	"net/http/httptrace"
	"sync"
)

type HttpConnTracking struct {
	// mu protects the values below
	mu sync.RWMutex
	// Thread-safe access to these fields is provided via getter methods.
	// Direct field access may not be thread-safe during concurrent HTTP operations.
	// Deprecated: Use GetTotalLatency() for thread-safe access
	TotalLatency string
	// Deprecated: Use GetDnsLatency() for thread-safe access
	DnsLatency string
	// Deprecated: Use GetConnLatency() for thread-safe access
	ConnLatency string
	// Deprecated: Use GetTlsLatency() for thread-safe access
	TlsLatency string
	// Deprecated: Use GetProtocol() for thread-safe access
	Protocol string
	// Deprecated: Use GetReqConnInfo() for thread-safe access
	ReqConnInfo *httptrace.GotConnInfo
}

// GetTotalLatency returns the total latency in a thread-safe manner
func (h *HttpConnTracking) GetTotalLatency() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.TotalLatency
}

// GetDnsLatency returns the DNS latency in a thread-safe manner
func (h *HttpConnTracking) GetDnsLatency() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.DnsLatency
}

// GetConnLatency returns the connection latency in a thread-safe manner
func (h *HttpConnTracking) GetConnLatency() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ConnLatency
}

// GetTlsLatency returns the TLS latency in a thread-safe manner
func (h *HttpConnTracking) GetTlsLatency() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.TlsLatency
}

// GetProtocol returns the negotiated protocol in a thread-safe manner
func (h *HttpConnTracking) GetProtocol() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.Protocol
}

// GetReqConnInfo returns the connection info in a thread-safe manner
func (h *HttpConnTracking) GetReqConnInfo() *httptrace.GotConnInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ReqConnInfo
}

func (h *HttpConnTracking) setTotalLatency(latency string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.TotalLatency = latency
}

func (h *HttpConnTracking) setDnsLatency(latency string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.DnsLatency = latency
}

func (h *HttpConnTracking) setConnLatency(latency string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ConnLatency = latency
}

func (h *HttpConnTracking) setTlsLatency(latency string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.TlsLatency = latency
}

func (h *HttpConnTracking) setProtocol(protocol string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Protocol = protocol
}

func (h *HttpConnTracking) setReqConnInfo(info *httptrace.GotConnInfo) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ReqConnInfo = info
}