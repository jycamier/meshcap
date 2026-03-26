package meshcap

import "testing"

func TestExtractTraceID(t *testing.T) {
	tests := []struct {
		name        string
		traceparent string
		want        string
	}{
		{
			name:        "valid traceparent",
			traceparent: "00-abcdef1234567890abcdef1234567890-1234567890abcdef-01",
			want:        "abcdef1234567890abcdef1234567890",
		},
		{
			name:        "empty string",
			traceparent: "",
			want:        "",
		},
		{
			name:        "no dashes",
			traceparent: "malformed",
			want:        "",
		},
		{
			name:        "only version",
			traceparent: "00",
			want:        "",
		},
		{
			name:        "version and trace_id only",
			traceparent: "00-traceid",
			want:        "traceid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTraceID(tt.traceparent)
			if got != tt.want {
				t.Errorf("ExtractTraceID(%q) = %q, want %q", tt.traceparent, got, tt.want)
			}
		})
	}
}
