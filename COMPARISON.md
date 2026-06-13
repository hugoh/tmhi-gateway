# HTTP Client Library Comparison

Candidates for `tmhi-gateway`: **resty v2** (current), **resty v3**, **net/http** (stdlib).

---

## What This Project Actually Uses from resty

| Feature | Call-sites |
|---|---|
| `SetBaseURL` / `SetTimeout` / `SetRetryCount` / `SetDebug` | gateway.go |
| `SetHeader` / `SetAuthToken` | gateway.go, arcadyan.go |
| `SetCookie` | nokia.go |
| `.R().SetContext().SetBody().Post()` + JSON marshal | arcadyan.go, nokia.go |
| `.R().SetContext().SetFormData().Post()` | nokia.go |
| `.R().SetContext().SetResult().Get()` + JSON unmarshal | arcadyan.go, nokia.go |
| `.R().SetContext().Execute(method, path)` | arcadyan.go |
| `.R().SetContext().Head()` | gateway.go |
| `resp.IsError()` / `resp.IsSuccess()` / `resp.StatusCode()` / `resp.Body()` / `resp.String()` / `resp.Header()` | all files |
| `NewWithClient(&http.Client{})` (test injection) | all `_test.go` files |
| `client.BaseURL` (direct field, for test assertion) | gateway_test.go |

---

## Metrics

| | resty v2 (current) | resty v3 | net/http |
|---|---|---|---|
| **Wrapper code needed** | 0 LOC | 0 LOC | **219 LOC** (`comparison/nethttp_client.go`) |
| **External deps added** | `resty/v2` | `resty/v3` | none |
| **Indirect deps via library** | `golang.org/x/net`, `golang.org/x/time` | same or similar | none |
| **go.sum entries** | 16 | ~16 | 10 (testify only) |
| **JSON body marshal** | built-in | built-in | manual (`json.Marshal` + `bytes.NewReader`) |
| **JSON result unmarshal** | built-in | built-in + generics | manual (`json.Unmarshal`) |
| **Retry with backoff** | built-in | built-in | manual (~20 LOC) |
| **Auth token header** | built-in | built-in | manual (map entry) |
| **Form data encoding** | built-in | built-in | manual (`url.Values`) |
| **Debug logging** | built-in | built-in | manual (custom Transport) |
| **Test client injection** | `NewWithClient(hc)` | `NewWithClient(hc)` | stdlib `*http.Client` directly |
| **API stability** | stable/GA | beta (not GA as of 2025) | stdlib (guaranteed) |
| **Migration cost from current** | none | moderate (breaking API changes at all call-sites) | high |

---

## resty v2 → v3: What Changes

The main addition in v3 is generics for type-safe result binding:

```go
// v2 (current)
var result LoginResp
resp, err := client.R().SetResult(&result).Post(path)

// v3
resp, err := resty.SetResult[LoginResp](client.R()).Post(path)
// or the expected idiomatic form depending on final v3 API
```

Other differences:
- Module path changes: `go-resty/resty/v3`
- Some middleware/hook API is redesigned
- Not GA as of the time of this comparison — no stable release tag

For this project the generics benefit is minor: the result structs are all local and well-typed already. The migration would touch every `.SetResult()` call site across `arcadyan.go` and `nokia.go` plus all test helpers for no meaningful runtime gain.

---

## net/http: What the Wrapper Requires

See `comparison/nethttp_client.go` for the concrete implementation. Key cost:

- **219 LOC** of wrapper code that now lives in *this repo* and must be maintained
- Retry logic is non-trivial to get right (backoff, retryable-vs-non-retryable conditions, context cancellation interaction)
- `resty.NewWithClient(hc)` used in tests maps 1:1 — the wrapper replicates this, but it's another surface to keep correct

The gain is removing two transitive deps (`golang.org/x/net` and `golang.org/x/time`), which are stable and low-risk google.golang.org packages.

---

## Recommendation

**Stay on resty v2.**

- The 219 LOC wrapper needed for net/http has to be written, tested, and maintained by this repo — that's a worse trade than a single stable external dep
- resty v3 is not GA; the generics benefit is cosmetic for this codebase; migration touches every call-site
- resty v2 maps cleanly to what the project needs: retry, JSON binding, form data, auth headers, test injection — all without boilerplate
