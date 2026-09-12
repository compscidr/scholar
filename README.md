# scholar
scholar is a Go module that fetches an author's publications — title, authors, venue, date, citation
count, links — from **Google Scholar** (by scraping the profile page) or from the **Semantic Scholar
API**, and caches them in memory and on disk. Its types can be used independently, and
`scholar-example/` is a small command-line tool built on them.

This tool is inspired by [scholar.py](https://github.com/ckreibich/scholar.py)

# Usage
```go
import scholar "github.com/compscidr/scholar"

sch := scholar.New("profiles.json", "articles.json")

// Optional: configure the delay between requests (default is 2 seconds)
sch.SetRequestDelay(1 * time.Second)

// Serves from cache when fresh, refreshes when stale, falls back to stale data on failure
articles, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 50)
if err != nil {
	// nothing cached and the fetch failed; see "Blocked IPs" below
}
for _, article := range articles {
	// do something with the article
}

// Persist the caches so the next process start doesn't need the network
sch.SaveCache("profiles.json", "articles.json")
```
`QueryProfile` fetches without consulting the cache. The command-line tool:
```bash
cd scholar-example && go build
./scholar-example -user SbUmSEAAAAAJ -limit 10
./scholar-example -source semantic_scholar -user 1792904 -limit 10
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
works the same for both sources. The default source remains Google Scholar.

Request volume: a listing is one request per 100 papers and already carries full details, so a
first fetch or a profile refresh needs no per-paper requests; new papers are seeded from the
listing. Individual `/paper/{id}` requests happen only when a cached article has expired (30 days)
and is being refreshed. Unauthenticated requests share a pool of roughly 100 requests per 5
minutes, so a daily or weekly refresh is well within that; set an API key if you need more.

## Features
* Two publication sources behind one API: Google Scholar (profile page scraping, the default) and
  the Semantic Scholar Academic Graph API
* Google Scholar: parses the profile page for the listing, then each article page for details
  (paginated, configurable limit)
* Semantic Scholar: one paginated API request per 100 papers, details included
* In-memory cache — profiles for 7 days, articles for 30 days — with stale data served when a
  refresh fails
* On-disk cache files (`SaveCache`) loaded on `New`, so a restart doesn't need the network
* Throttling with a configurable delay between requests, and exponential-backoff retry on 429
* `ErrBlocked` for Google's "automated queries" 403, and a per-user failure cooldown so an
  empty-cache consumer doesn't retry on every call

## Testing

The module includes mocked tests (fast, no network) and optional integration tests against the real
Google Scholar site. Google Scholar tests use the recorded `sample_author_page.html` /
`sample_article_page.html`; Semantic Scholar tests use JSON responses recorded from the real API in
`testdata/` (author 1792904, 47 papers, split into two pages to exercise pagination).

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