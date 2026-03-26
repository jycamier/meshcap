package meshcap

import "strings"

// ExtractTraceID parses a W3C traceparent header (version-trace_id-parent_id-trace_flags)
// and returns the trace_id segment. Returns empty string if the header is empty or malformed.
func ExtractTraceID(traceparent string) string {
	if parts := strings.SplitN(traceparent, "-", 4); len(parts) >= 2 {
		return parts[1]
	}
	return ""
}
