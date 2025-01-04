package rpc

import "net/http"

type responseWrapper struct {
	wrapped http.ResponseWriter
	// TODO: guarding reads necessary?
	responseSize int
	status       int
}

func (rw *responseWrapper) Flush() {
	if flusher, ok := rw.wrapped.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *responseWrapper) Write(data []byte) (int, error) {
	size, err := rw.wrapped.Write(data)
	rw.responseSize += size
	return size, err
}

func (rw *responseWrapper) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.wrapped.WriteHeader(statusCode)
}

func (rw *responseWrapper) Header() http.Header {
	return rw.wrapped.Header()
}
