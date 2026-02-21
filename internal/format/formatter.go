package format

import "github.com/jycamier/meshcap/internal/model"

// Formatter serializes HTTP request records into a byte format.
type Formatter interface {
	Format(records []model.HTTPRequest) ([]byte, error)
	Extension() string
}
