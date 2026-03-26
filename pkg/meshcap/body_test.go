package meshcap

import "testing"

func TestLimitBody(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		maxSize int
		wantLen int
	}{
		{
			name:    "under limit",
			body:    []byte("hello"),
			maxSize: 10,
			wantLen: 5,
		},
		{
			name:    "at limit",
			body:    []byte("hello"),
			maxSize: 5,
			wantLen: 5,
		},
		{
			name:    "over limit",
			body:    []byte("hello world"),
			maxSize: 5,
			wantLen: 5,
		},
		{
			name:    "empty body",
			body:    nil,
			maxSize: 10,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LimitBody(tt.body, tt.maxSize)
			if len(got) != tt.wantLen {
				t.Errorf("LimitBody() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestAppendBodyChunk(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		chunk   []byte
		maxSize int
		wantLen int
	}{
		{
			name:    "append within limit",
			buf:     []byte("he"),
			chunk:   []byte("llo"),
			maxSize: 10,
			wantLen: 5,
		},
		{
			name:    "chunk truncated to fit",
			buf:     []byte("hel"),
			chunk:   []byte("lo world"),
			maxSize: 5,
			wantLen: 5,
		},
		{
			name:    "buffer already full",
			buf:     []byte("hello"),
			chunk:   []byte(" world"),
			maxSize: 5,
			wantLen: 5,
		},
		{
			name:    "empty chunk",
			buf:     []byte("he"),
			chunk:   nil,
			maxSize: 10,
			wantLen: 2,
		},
		{
			name:    "empty buffer",
			buf:     nil,
			chunk:   []byte("hello"),
			maxSize: 3,
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AppendBodyChunk(tt.buf, tt.chunk, tt.maxSize)
			if len(got) != tt.wantLen {
				t.Errorf("AppendBodyChunk() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
