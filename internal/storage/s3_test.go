package storage

import (
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

func TestBuildKey(t *testing.T) {
	key := BuildKey("captures", "example.com", testTime, ".parquet")

	if !strings.HasPrefix(key, "captures/host=example.com/year=2025/month=06/day=15/hour=14/") {
		t.Errorf("unexpected prefix: %s", key)
	}
	if !strings.HasSuffix(key, ".parquet") {
		t.Errorf("expected .parquet suffix: %s", key)
	}
}

func TestBuildKeyHARExtension(t *testing.T) {
	key := BuildKey("captures", "example.com", testTime, ".har")

	if !strings.HasSuffix(key, ".har") {
		t.Errorf("expected .har suffix: %s", key)
	}
	if !strings.Contains(key, "host=example.com") {
		t.Errorf("expected host partition: %s", key)
	}
}

func TestBuildKeyNoPrefix(t *testing.T) {
	key := BuildKey("", "example.com", testTime, ".parquet")

	if strings.HasPrefix(key, "/") {
		t.Errorf("empty prefix should not produce leading slash: %s", key)
	}
	if !strings.HasPrefix(key, "host=example.com/") {
		t.Errorf("expected key to start with host partition: %s", key)
	}
}

func TestBuildKeySpecialCharsInHost(t *testing.T) {
	key := BuildKey("data", "host:8080/path?q=1", testTime, ".parquet")

	if !strings.Contains(key, "host=host_8080_path_q_1") {
		t.Errorf("expected sanitized host: %s", key)
	}
}

func TestBuildKeyEmptyHost(t *testing.T) {
	key := BuildKey("data", "", testTime, ".parquet")

	if !strings.Contains(key, "host=_unknown_") {
		t.Errorf("expected _unknown_ for empty host: %s", key)
	}
}

func TestBuildKeyTrailingSlashPrefix(t *testing.T) {
	key := BuildKey("prefix/", "example.com", testTime, ".parquet")

	if strings.Contains(key, "//") {
		t.Errorf("should not contain double slash: %s", key)
	}
	if !strings.HasPrefix(key, "prefix/host=") {
		t.Errorf("expected trimmed prefix: %s", key)
	}
}

func TestBuildKeyUniqueUUIDs(t *testing.T) {
	key1 := BuildKey("data", "example.com", testTime, ".parquet")
	key2 := BuildKey("data", "example.com", testTime, ".parquet")

	if key1 == key2 {
		t.Errorf("expected unique keys, got identical: %s", key1)
	}
}
