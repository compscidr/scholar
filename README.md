# scholar
scholar is a WiP Go module that implements a querier and parser for Google Scholar's output. Its classes can be used 
independently, but it can also be invoked as a command-line tool.

This tool is inspired by [scholar.py](https://github.com/ckreibich/scholar.py)

# Usage
```
import "github.com/compscidr/scholar"

sch := scholar.New("profiles.json", "articles.json")

// Optional: Configure request delay for throttling (default is 2 seconds)
sch.SetRequestDelay(1 * time.Second)

articles := sch.QueryProfile("SbUmSEAAAAAJ", 1)

for _, article := range articles {
	// do something with the article
}
```

## Semantic Scholar source
Google Scholar refuses requests from most cloud/datacenter IPs (see *Blocked IPs* below). As an
alternative the library can read the same data from the
[Semantic Scholar Academic Graph API](https://api.semanticscholar.org/api-docs/graph), which is a real
API with no IP blocking. Coverage and citation counts differ from Google Scholar (some publications
are missing and counts are generally lower), and users are identified by their Semantic Scholar
**author id** (the number at the end of `https://www.semanticscholar.org/author/<name>/<id>`), not the
Google Scholar id.
```go
sch := scholar.New("profiles.json", "articles.json")
sch.SetSource(scholar.SourceSemanticScholar)
sch.SetAPIKey(os.Getenv("S2_API_KEY")) // optional; raises the rate limit

articles, err := sch.QueryProfileWithMemoryCache("1792904", 50)
```
Everything else — `Article` fields, the on-disk cache files, throttling and the failure cooldown —
works the same for both sources. The default source remains Google Scholar. Unauthenticated
requests share a pool of roughly 100 requests per 5 minutes; a profile fetch is one request per 100
papers, so a daily or weekly refresh is well within that.

## Features
Working:
* Queries and parses a user profile by user id to get basic publication data
* Queries each of the articles listed (up to 80) and parses the results for extra information
* Caches the profile for a day, and articles for a week (need to confirm this is working)
  * This is in memory, so if the program is restarted, the cache is lost
* Configurable limit to number of articles to query in one go
* On-disk caching of the profile and articles to avoid hitting the rate limit
* **Rate limiting and throttling with configurable delays between requests**
* **Automatic retry with exponential backoff for 429 (Too Many Requests) responses**

## Testing

The module includes both mocked tests (fast, no network) and optional integration tests (against real Google Scholar API).

### Running Tests

```bash
# Run all tests (uses mock HTTP client, no network requests)
go test

# Run specific test
go test -run TestProfileQuerier

# Run integration tests against real Google Scholar API (optional)
go test -tags integration

# Note: Integration tests may fail due to rate limits or network restrictions
# This is expected and will not break CI/CD pipelines
```

The integration tests are designed to be optional - they test against the real Google Scholar API but gracefully handle network failures and rate limits. This allows developers to verify functionality against the live API when needed without breaking automated builds.

## TODO:
* Pagination of articles

## Rate Limiting
The library automatically throttles requests to avoid hitting Google Scholar's rate limits:
* Default delay: 2 seconds between requests
* Configurable via `SetRequestDelay(duration)`
* Automatic retry with exponential backoff for 429 responses (up to 3 retries)
* Backoff delays: 5s, 10s, 20s for subsequent retries

## Blocked IPs and Failure Cooldown
Google Scholar refuses requests from IPs it believes send automated queries (cloud/datacenter
ranges are commonly affected). Such responses are a `403` with a "Sorry..." page rather than a
`429`, so they are not retried. The library reports them as `ErrBlocked`:
```go
articles, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 50)
if errors.Is(err, go_scholar.ErrBlocked) {
    // this IP is blocked; retrying will not help
}
```
When a fetch fails for a user with **no cached data**, the library will not contact Google again for
that user until a cooldown has passed (default 1 hour), returning the original error in the meantime.
This stops an empty-cache consumer from turning every call into a new request:
```go
sch.SetFailureCooldown(10 * time.Minute) // or 0 to disable
```
When cached data exists, a failed refresh falls back to the stale cache as before.

## Possible throttle info:
https://stackoverflow.com/questions/60271587/how-long-is-the-error-429-toomanyrequests-cooldown