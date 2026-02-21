package storage

import (
	"context"
	"testing"

	appconfig "github.com/jycamier/meshcap/internal/config"
)

func TestNewAzureBlobStorageValidation(t *testing.T) {
	cfg := &appconfig.Config{
		AzureAccount:   "",
		AzureContainer: "test",
	}

	_, err := NewAzureBlobStorage(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("expected error for empty account")
	}
}

func TestNewAzureBlobStorageValidationContainer(t *testing.T) {
	cfg := &appconfig.Config{
		AzureAccount:   "myaccount",
		AzureContainer: "",
	}

	_, err := NewAzureBlobStorage(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("expected error for empty container")
	}
}
