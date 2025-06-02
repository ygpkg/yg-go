package estool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// UpdateESFromReader updates ES with embeddings from a io.Reader
func UpdateESFromReader(ctx context.Context, escli *elasticsearch.Client, index string, f io.Reader) error {
	req := esapi.BulkRequest{
		Index: index,
		Body:  f,
	}
	res, err := req.Do(ctx, escli)
	if err != nil {
		return fmt.Errorf("error executing the bulk request: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("bulk update failed: %s", body)
	}

	var bulkResponse ResponseBody
	if err := json.NewDecoder(res.Body).Decode(&bulkResponse); err != nil {
		return fmt.Errorf("error parsing the response body: %w", err)
	}

	if bulkResponse.HasError {
		errs := bulkResponse.Errors()
		return fmt.Errorf("bulk update contained errors: %v", errs)
	}

	return nil
}
