package format

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jycamier/meshcap/internal/model"
)

// HARFormatter produces HAR 1.2 JSON documents from HTTP request records.
type HARFormatter struct{}

// NewHARFormatter returns a new HARFormatter.
func NewHARFormatter() *HARFormatter {
	return &HARFormatter{}
}

// HAR document structure types (private, used only for JSON marshaling).

type harDoc struct {
	Log harLogObj `json:"log"`
}

type harLogObj struct {
	Version string        `json:"version"`
	Creator harCreatorObj `json:"creator"`
	Entries []harEntryObj `json:"entries"`
}

type harCreatorObj struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harEntryObj struct {
	StartedDateTime string        `json:"startedDateTime"`
	Request         harRequestObj `json:"request"`
	Response        harResponseObj `json:"response"`
}

type harRequestObj struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	HTTPVersion string         `json:"httpVersion"`
	Headers     []harHeaderObj `json:"headers"`
	BodySize    int64          `json:"bodySize"`
	PostData    *harPostData   `json:"postData,omitempty"`
}

type harHeaderObj struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type harResponseObj struct {
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
}

// Format serializes HTTP request records into a HAR 1.2 JSON document.
func (f *HARFormatter) Format(records []model.HTTPRequest) ([]byte, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("no records to write")
	}

	entries := make([]harEntryObj, 0, len(records))
	for _, r := range records {
		entry := harEntryObj{
			StartedDateTime: r.CapturedAt,
			Request: harRequestObj{
				Method:      r.ReqMethod,
				URL:         "http://" + r.ReqHost + r.ReqPath,
				HTTPVersion: r.ReqVersion,
				Headers:     parseHeaders(r.ReqHeaders),
				BodySize:    r.ReqBodySize,
			},
			Response: harResponseObj{
				Status:     0,
				StatusText: "",
			},
		}

		if len(r.ReqBody) > 0 {
			mimeType := extractMimeType(r.ReqHeaders)
			entry.Request.PostData = &harPostData{
				MimeType: mimeType,
				Text:     string(r.ReqBody),
			}
		}

		entries = append(entries, entry)
	}

	doc := harDoc{
		Log: harLogObj{
			Version: "1.2",
			Creator: harCreatorObj{
				Name:    "meshcap",
				Version: "1.0",
			},
			Entries: entries,
		},
	}

	return json.Marshal(doc)
}

// Extension returns the file extension for HAR files.
func (f *HARFormatter) Extension() string {
	return ".har"
}

// parseHeaders parses a JSON string of headers into a sorted slice of harHeaderObj.
// If the JSON is invalid, it returns an empty slice.
func parseHeaders(headersJSON string) []harHeaderObj {
	var raw map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &raw); err != nil {
		return []harHeaderObj{}
	}

	headers := make([]harHeaderObj, 0, len(raw))
	for name, value := range raw {
		headers = append(headers, harHeaderObj{Name: name, Value: value})
	}

	sort.Slice(headers, func(i, j int) bool {
		return headers[i].Name < headers[j].Name
	})

	return headers
}

// extractMimeType extracts the MIME type from a JSON headers string.
// It looks for the Content-Type header and returns the MIME type portion.
// If not found, it returns "application/octet-stream".
func extractMimeType(headersJSON string) string {
	var raw map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &raw); err != nil {
		return "application/octet-stream"
	}

	for name, value := range raw {
		if strings.EqualFold(name, "Content-Type") {
			// Take only the mime type part (before any ;params)
			parts := strings.SplitN(value, ";", 2)
			return strings.TrimSpace(parts[0])
		}
	}

	return "application/octet-stream"
}
