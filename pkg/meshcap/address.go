package meshcap

// StripPort removes the port suffix from an address like "10.0.0.1:1234".
func StripPort(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
