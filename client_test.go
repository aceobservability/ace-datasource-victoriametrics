package victoriametrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNew_requiresHTTPClient(t *testing.T) {
	t.Parallel()

	client, err := New("http://localhost:8428", nil)
	if err == nil {
		t.Fatal("expected error for nil http client")
	}
	if client != nil {
		t.Fatal("expected nil client when http client is missing")
	}
	if !strings.Contains(err.Error(), "http client is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryAndTestConnection_againstFixtureHTTP(t *testing.T) {
	t.Parallel()

	var sawQueryRange, sawHealth, sawLabels, sawMetricNames, sawLabelValues bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/api/v1/query_range"):
			sawQueryRange = true
			if got := r.URL.Query().Get("query"); got != "up" {
				t.Errorf("query_range query=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up","job":"vm"},"values":[[1600000000,"1"]]}]}}`))
		case r.URL.Path == "/health":
			sawHealth = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case r.URL.Path == "/api/v1/labels":
			sawLabels = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":["__name__","job"]}`))
		case r.URL.Path == "/api/v1/label/__name__/values":
			sawMetricNames = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":["up","vm_rows"]}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/label/") && strings.HasSuffix(r.URL.Path, "/values"):
			sawLabelValues = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":["vm","node"]}`))
		case strings.HasSuffix(r.URL.Path, "/api/v1/query"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Unix(1600000000, 0).Add(-time.Hour)
	end := time.Unix(1600000000, 0)
	result, err := client.Query(ctx, "up", start, end, time.Minute, 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("Query status=%q error=%q", result.Status, result.Error)
	}
	if result.ResultType != "metrics" {
		t.Fatalf("ResultType=%q, want metrics", result.ResultType)
	}
	if result.Data == nil || len(result.Data.Result) != 1 {
		t.Fatalf("expected 1 series, got %+v", result.Data)
	}
	if result.Data.Result[0].Metric["job"] != "vm" {
		t.Fatalf("metric labels=%v", result.Data.Result[0].Metric)
	}
	if !sawQueryRange {
		t.Fatal("expected fixture to receive /api/v1/query_range")
	}

	labels, err := client.Labels(ctx, "")
	if err != nil {
		t.Fatalf("Labels: %v", err)
	}
	if len(labels) != 2 || labels[0] != "__name__" || labels[1] != "job" {
		t.Fatalf("Labels=%v", labels)
	}
	if !sawLabels {
		t.Fatal("expected Labels to hit fixture /api/v1/labels")
	}

	names, err := client.MetricNames(ctx, "")
	if err != nil {
		t.Fatalf("MetricNames: %v", err)
	}
	if len(names) != 2 || names[0] != "up" {
		t.Fatalf("MetricNames=%v", names)
	}
	if !sawMetricNames {
		t.Fatal("expected MetricNames to hit fixture /api/v1/label/__name__/values")
	}

	filtered, err := client.MetricNames(ctx, "VM_")
	if err != nil {
		t.Fatalf("MetricNames search: %v", err)
	}
	if len(filtered) != 1 || filtered[0] != "vm_rows" {
		t.Fatalf("MetricNames search=%v", filtered)
	}

	values, err := client.LabelValues(ctx, "job", "")
	if err != nil {
		t.Fatalf("LabelValues: %v", err)
	}
	if len(values) != 2 || values[0] != "vm" {
		t.Fatalf("LabelValues=%v", values)
	}
	if !sawLabelValues {
		t.Fatal("expected LabelValues to hit fixture /api/v1/label/job/values")
	}

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !sawHealth {
		t.Fatal("expected TestConnection to hit /health")
	}
}

func TestQuery_statusErrorReturnsResult(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"error","error":"bad_data: invalid query"}`))
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Query(ctx, "not a query", time.Now().Add(-time.Hour), time.Now(), time.Minute, 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Status != "error" {
		t.Fatalf("status=%q, want error", result.Status)
	}
	if result.Error != "bad_data: invalid query" {
		t.Fatalf("error=%q", result.Error)
	}
	if result.ResultType != "metrics" {
		t.Fatalf("ResultType=%q", result.ResultType)
	}
}

func TestTestConnection_usesHealthThenQueryFallback(t *testing.T) {
	t.Parallel()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.Contains(r.URL.Path, "/api/v1/query") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"scalar","result":[1,"1"]}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if len(paths) < 2 || paths[0] != "/health" {
		t.Fatalf("paths=%v, want /health then query", paths)
	}
}
