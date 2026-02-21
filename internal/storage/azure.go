package storage

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	appconfig "github.com/jycamier/meshcap/internal/config"
)

type AzureBlobStorage struct {
	client    *azblob.Client
	container string
	logger    *slog.Logger
}

func NewAzureBlobStorage(ctx context.Context, cfg *appconfig.Config, logger *slog.Logger) (*AzureBlobStorage, error) {
	if cfg.AzureAccount == "" {
		return nil, fmt.Errorf("azure-account is required")
	}
	if cfg.AzureContainer == "" {
		return nil, fmt.Errorf("azure-container is required")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azure credential: %w", err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", cfg.AzureAccount)
	client, err := azblob.NewClient(serviceURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure blob client: %w", err)
	}

	return &AzureBlobStorage{
		client:    client,
		container: cfg.AzureContainer,
		logger:    logger,
	}, nil
}

func (a *AzureBlobStorage) Upload(ctx context.Context, key string, data []byte) error {
	_, err := a.client.UploadBuffer(ctx, a.container, key, data, &azblob.UploadBufferOptions{})
	if err != nil {
		return fmt.Errorf("azure upload %q: %w", key, err)
	}

	if a.logger != nil {
		a.logger.Info("uploaded file to Azure Blob", "key", key, "size", len(data), "container", a.container)
	}
	return nil
}
