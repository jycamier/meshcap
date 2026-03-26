package meshcap

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

func newHTTPDispatcher(collectorURL string, timeout time.Duration) DispatchFunc {
	client := &http.Client{Timeout: timeout}
	return func(req DispatchRequest) error {
		httpReq, err := http.NewRequest(req.Method, collectorURL, bytes.NewReader(req.Payload))
		if err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			return fmt.Errorf("collector returned status %d", resp.StatusCode)
		}
		return nil
	}
}
