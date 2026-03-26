package meshcap

import "testing"

func TestStripPort(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{
			name: "ipv4 with port",
			addr: "10.0.0.1:1234",
			want: "10.0.0.1",
		},
		{
			name: "ipv4 without port",
			addr: "10.0.0.1",
			want: "10.0.0.1",
		},
		{
			name: "hostname with port",
			addr: "example.com:8080",
			want: "example.com",
		},
		{
			name: "empty string",
			addr: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripPort(tt.addr)
			if got != tt.want {
				t.Errorf("StripPort(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
