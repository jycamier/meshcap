package meshcap

import "testing"

func TestMarshalHeaders(t *testing.T) {
	tests := []struct {
		name string
		h    map[string]string
		want string
	}{
		{
			name: "single header",
			h:    map[string]string{"content-type": "application/json"},
			want: `{"content-type":"application/json"}`,
		},
		{
			name: "empty map",
			h:    map[string]string{},
			want: `{}`,
		},
		{
			name: "nil map",
			h:    nil,
			want: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarshalHeaders(tt.h)
			if got != tt.want {
				t.Errorf("MarshalHeaders() = %q, want %q", got, tt.want)
			}
		})
	}
}
