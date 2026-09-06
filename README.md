# ace-datasource-victoriametrics

Compile-time VictoriaMetrics datasource module for [Ace](https://github.com/aceobservability/ace).

Ace keeps the datasource contract and registry in
`github.com/aceobservability/ace/backend/pkg/datasource`. This module implements
that `Client` (plus connection test and PromQL metadata). Ace registers the
factory at `init` and injects its SSRF-safe HTTP client. This module does not
import Ace `internal/` packages and does not construct an unpolicy'd client.

## Contract

| Surface | Package |
| --- | --- |
| Query / result types | `github.com/aceobservability/ace/backend/pkg/datasource` |
| Registry type key | `victoriametrics` (`Type`) |
| Factory | `New(url string, httpClient *http.Client)` |

`httpClient` is required. Ace passes `ssrf.DatasourceClient` wrapped with stored
datasource credentials.

Query uses GET `/api/v1/query_range`. Metadata uses `/api/v1/labels` and
`/api/v1/label/{}/values`.

## Tests

```
go test ./...
```

Query and connection tests speak to an `httptest` fixture. No live VictoriaMetrics
is required.
