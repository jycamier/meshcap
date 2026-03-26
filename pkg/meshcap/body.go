package meshcap

// LimitBody truncates body to maxSize bytes if it exceeds the limit.
func LimitBody(body []byte, maxSize int) []byte {
	if len(body) > maxSize {
		return body[:maxSize]
	}
	return body
}

// AppendBodyChunk appends a chunk to buf without exceeding maxSize.
// Used for streaming body capture (e.g. WASM OnHttpRequestBody callbacks).
func AppendBodyChunk(buf []byte, chunk []byte, maxSize int) []byte {
	remaining := maxSize - len(buf)
	if remaining <= 0 {
		return buf
	}
	if len(chunk) > remaining {
		chunk = chunk[:remaining]
	}
	return append(buf, chunk...)
}
