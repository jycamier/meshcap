package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Storage uploads serialized data to a backend.
type Storage interface {
	Upload(ctx context.Context, key string, data []byte) error
}

// BuildKey creates a Hive-partitioned key.
// Format: {prefix}/host={host}/year=YYYY/month=MM/day=DD/hour=HH/{uuid}{ext}
func BuildKey(prefix string, host string, t time.Time, ext string) string {
	safeHost := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, host)

	if safeHost == "" {
		safeHost = "_unknown_"
	}

	key := fmt.Sprintf("host=%s/year=%04d/month=%02d/day=%02d/hour=%02d/%s%s",
		safeHost,
		t.Year(), t.Month(), t.Day(), t.Hour(),
		uuid.New().String(),
		ext,
	)

	if prefix != "" {
		prefix = strings.TrimRight(prefix, "/")
		key = prefix + "/" + key
	}

	return key
}
