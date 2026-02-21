package format

import (
	"bytes"
	"fmt"

	"github.com/jycamier/meshcap/internal/model"
	"github.com/parquet-go/parquet-go"
)

type ParquetFormatter struct{}

func NewParquetFormatter() *ParquetFormatter {
	return &ParquetFormatter{}
}

func (f *ParquetFormatter) Format(records []model.HTTPRequest) ([]byte, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("no records to write")
	}

	var buf bytes.Buffer
	w := parquet.NewGenericWriter[model.HTTPRequest](&buf)

	if _, err := w.Write(records); err != nil {
		return nil, fmt.Errorf("parquet write: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("parquet close: %w", err)
	}

	return buf.Bytes(), nil
}

func (f *ParquetFormatter) Extension() string {
	return ".parquet"
}
