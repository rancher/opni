package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func AtMostOne[Resp any](ctx context.Context, client *http.Client, reqs []*http.Request) (*Resp, error) {
	var retErr error
	for _, req := range reqs {
		var workingResp Resp
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			retErr = err
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			retErr = fmt.Errorf("unexpected status code %d", resp.StatusCode)
			continue
		}
		if err := json.NewDecoder(resp.Body).Decode(&workingResp); err != nil {
			retErr = err
			continue
		}
		return &workingResp, nil
	}
	return nil, retErr
}
