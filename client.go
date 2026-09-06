package victoriametrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/aceobservability/ace/backend/pkg/datasource"
)

// Type is the RegisterDatasource key Ace uses for this module.
const Type = "victoriametrics"

// Client implements the Ace VictoriaMetrics query datasource.
type Client struct {
	url        string
	httpClient *http.Client
	meta       *promqlMetadata
}

// New constructs a VictoriaMetrics datasource client.
// httpClient is required so Ace can inject DatasourceClient (dial/redirect policy + auth).
func New(baseURL string, httpClient *http.Client) (*Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("http client is required")
	}
	return &Client{
		url:        baseURL,
		httpClient: httpClient,
		meta: &promqlMetadata{
			baseURL: baseURL,
			client:  httpClient,
		},
	}, nil
}

// HTTPClient returns the injected HTTP client. Ace SSRF tests inspect policy wiring.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

type vmQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

func (c *Client) Query(ctx context.Context, query string, start, end time.Time, step time.Duration, limit int) (*datasource.QueryResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(start.Unix(), 10))
	params.Set("end", strconv.FormatInt(end.Unix(), 10))
	params.Set("step", fmt.Sprintf("%ds", int(step.Seconds())))

	reqURL := fmt.Sprintf("%s/api/v1/query_range?%s", c.url, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query VictoriaMetrics: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var vmResp vmQueryResponse
	if err := json.Unmarshal(body, &vmResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if vmResp.Status != "success" {
		return &datasource.QueryResult{
			Status:     "error",
			Error:      vmResp.Error,
			ResultType: "metrics",
		}, nil
	}

	result := &datasource.QueryResult{
		Status:     "success",
		ResultType: "metrics",
		Data: &datasource.QueryData{
			ResultType: vmResp.Data.ResultType,
			Result:     make([]datasource.MetricResult, len(vmResp.Data.Result)),
		},
	}

	for i, r := range vmResp.Data.Result {
		result.Data.Result[i] = datasource.MetricResult{
			Metric: r.Metric,
			Values: r.Values,
		}
	}

	return result, nil
}

func (c *Client) Labels(ctx context.Context, metric string) ([]string, error) {
	return c.meta.Labels(ctx, metric)
}

func (c *Client) LabelValues(ctx context.Context, label, metric string) ([]string, error) {
	return c.meta.LabelValues(ctx, label, metric)
}

func (c *Client) MetricNames(ctx context.Context, search string) ([]string, error) {
	return c.meta.MetricNames(ctx, search)
}

var (
	_ datasource.Client                  = (*Client)(nil)
	_ datasource.MetricLabelsClient      = (*Client)(nil)
	_ datasource.MetricLabelValuesClient = (*Client)(nil)
	_ datasource.MetricNamesClient       = (*Client)(nil)
)
