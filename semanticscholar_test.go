package go_scholar

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockSemanticScholarClient serves the recorded fixtures for author 1792904
// and records every request it sees.
type MockSemanticScholarClient struct {
	Requests []*http.Request
}

func (m *MockSemanticScholarClient) Do(req *http.Request) (*http.Response, error) {
	m.Requests = append(m.Requests, req)
	serve := func(name string) (*http.Response, error) {
		content, err := os.ReadFile("testdata/" + name)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(content)))}, nil
	}
	q := req.URL.Query()
	switch {
	case req.URL.Path == "/graph/v1/author/1792904/papers" && q.Get("offset") == "0":
		return serve("semantic_scholar_papers_page1.json")
	case req.URL.Path == "/graph/v1/author/1792904/papers" && q.Get("offset") == "30":
		return serve("semantic_scholar_papers_page2.json")
	case req.URL.Path == "/graph/v1/paper/d87ccdd7ac0f2a4a953cc680ed1e54c2da87f42b":
		return serve("semantic_scholar_paper.json")
	}
	return &http.Response{StatusCode: 404, Status: "404 Not Found", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"not found"}`))}, nil
}

type cachePaths struct{ profiles, articles string }

func newSemanticScholar(t *testing.T) (*Scholar, *MockSemanticScholarClient) {
	sch, client, _ := newSemanticScholarWithPaths(t)
	return sch, client
}

func newSemanticScholarWithPaths(t *testing.T) (*Scholar, *MockSemanticScholarClient, cachePaths) {
	t.Helper()
	dir := t.TempDir()
	paths := cachePaths{profiles: dir + "/profiles.json", articles: dir + "/articles.json"}
	sch := New(paths.profiles, paths.articles)
	sch.SetRequestDelay(time.Millisecond)
	sch.SetSource(SourceSemanticScholar)
	client := &MockSemanticScholarClient{}
	sch.SetHTTPClient(client)
	return sch, client, paths
}

func TestSemanticScholar_QueryProfile_PaginatesAndMaps(t *testing.T) {
	sch, client := newSemanticScholar(t)

	articles, err := sch.QueryProfile("1792904", 100)
	assert.NoError(t, err)
	assert.Len(t, articles, 47, "both fixture pages should be consumed")
	assert.Len(t, client.Requests, 2, "one request per page, no per-article requests")

	// Every request asks for the fields we map and paginates by offset.
	for _, r := range client.Requests {
		assert.Equal(t, "api.semanticscholar.org", r.URL.Host)
		assert.Contains(t, r.URL.Query().Get("fields"), "citationCount")
		assert.Equal(t, "", r.Header.Get("x-api-key"), "no key configured, none sent")
	}
	assert.Equal(t, "30", client.Requests[1].URL.Query().Get("offset"))

	// Field mapping, using a paper with every field populated.
	var a *Article
	for _, art := range articles {
		if strings.HasPrefix(art.Title, "The Diverse Technology of MANETs") {
			a = art
		}
	}
	if assert.NotNil(t, a) {
		assert.Equal(t, "https://www.semanticscholar.org/paper/d87ccdd7ac0f2a4a953cc680ed1e54c2da87f42b", a.ScholarURL)
		assert.Contains(t, a.Authors, "F. Safari")
		assert.Contains(t, a.Authors, "Jason B. Ernst")
		assert.Equal(t, 2023, a.Year)
		assert.Equal(t, 6, a.Month)
		assert.Equal(t, 1, a.Day)
		assert.Equal(t, 23, a.NumCitations)
		assert.Equal(t, "International Journal of Future Computer and Communication", a.Journal)
		assert.Equal(t, "https://www.ijfcc.org/vol12/601-NC03.pdf", a.PdfURL)
		assert.False(t, a.LastRetrieved.IsZero())
	}

	// Edge cases from the real data: null publicationDate falls back to the
	// year; a CLOSED open-access entry has an empty PDF URL; a couple of
	// entries (theses, chapters) have neither journal nor venue and must
	// simply map with an empty Journal rather than fail.
	for _, art := range articles {
		if strings.HasPrefix(art.Title, "A Novel Cross-Layer Adaptive Fuzzy-Based") {
			assert.Equal(t, 2023, art.Year)
			assert.Equal(t, 0, art.Month)
			assert.Equal(t, "11", art.Volume)
			assert.Equal(t, "50805-50822", art.Pages)
		}
		if strings.HasPrefix(art.Title, "A Review of AI-based MANET Routing") {
			assert.Equal(t, "", art.PdfURL, "CLOSED access must not yield a PDF URL")
		}
	}
}

// Journal falls back to venue when the API has no journal object, and the
// paper URL is synthesised from paperId when the API omits it.
func TestSemanticScholar_ToArticleFallbacks(t *testing.T) {
	p := s2Paper{PaperID: "abc123", Title: "T", Venue: "Some Conference", Year: 2020, PublicationDate: "2020-05"}
	a := p.toArticle()
	assert.Equal(t, "Some Conference", a.Journal)
	assert.Equal(t, "https://www.semanticscholar.org/paper/abc123", a.ScholarURL)
	assert.Equal(t, 2020, a.Year)
	assert.Equal(t, 0, a.Month, "a partial date is ignored rather than misparsed")

	p.Journal = &struct {
		Name   string `json:"name"`
		Volume string `json:"volume"`
		Pages  string `json:"pages"`
	}{Name: "Journal Name", Volume: "3"}
	a = p.toArticle()
	assert.Equal(t, "Journal Name", a.Journal, "journal name wins over venue")
	assert.Equal(t, "3", a.Volume)
}

func TestSemanticScholar_LimitStopsPagination(t *testing.T) {
	sch, client := newSemanticScholar(t)
	articles, err := sch.QueryProfile("1792904", 10)
	assert.NoError(t, err)
	assert.Len(t, articles, 10)
	assert.Len(t, client.Requests, 1, "the limit fits in the first page; no second request")
}

func TestSemanticScholar_APIKeyHeader(t *testing.T) {
	sch, client := newSemanticScholar(t)
	sch.SetAPIKey("secret")
	_, err := sch.QueryProfile("1792904", 5)
	assert.NoError(t, err)
	assert.Equal(t, "secret", client.Requests[0].Header.Get("x-api-key"))
}

func TestSemanticScholar_MemoryCacheAndArticleRefresh(t *testing.T) {
	sch, client, paths := newSemanticScholarWithPaths(t)

	articles, err := sch.QueryProfileWithMemoryCache("1792904", 100)
	assert.NoError(t, err)
	assert.Len(t, articles, 47)
	assert.Len(t, client.Requests, 2)

	// Cache hit: no requests.
	articles, err = sch.QueryProfileWithMemoryCache("1792904", 100)
	assert.NoError(t, err)
	assert.Len(t, articles, 47)
	assert.Len(t, client.Requests, 2)

	// An expired article is refreshed from the paper endpoint.
	key := "https://www.semanticscholar.org/paper/d87ccdd7ac0f2a4a953cc680ed1e54c2da87f42b"
	cached, _ := sch.articles.Load(key)
	stale := *cached.(*Article)
	stale.LastRetrieved = time.Now().Add(-31 * 24 * time.Hour)
	sch.articles.Store(key, &stale)
	articles, err = sch.QueryProfileWithMemoryCache("1792904", 100)
	assert.NoError(t, err)
	assert.Len(t, articles, 47)
	assert.Len(t, client.Requests, 3)
	assert.Equal(t, "/graph/v1/paper/d87ccdd7ac0f2a4a953cc680ed1e54c2da87f42b", client.Requests[2].URL.Path)

	// The cache round-trips through the same files the Google source uses.
	sch.SaveCache(paths.profiles, paths.articles)
	reloaded := New(paths.profiles, paths.articles)
	reloaded.SetRequestDelay(time.Millisecond)
	reloaded.SetSource(SourceSemanticScholar)
	reloaded.SetHTTPClient(client)
	articles, err = reloaded.QueryProfileWithMemoryCache("1792904", 100)
	assert.NoError(t, err)
	assert.Len(t, articles, 47)
	assert.Len(t, client.Requests, 3, "a reloaded cache should not need the network")
}

func TestSemanticScholar_UnknownAuthorIsAnError(t *testing.T) {
	sch, _ := newSemanticScholar(t)
	_, err := sch.QueryProfile("does-not-exist", 10)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrBlocked))
	assert.Contains(t, err.Error(), "404")
}

func TestDefaultSourceIsGoogleScholar(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	assert.Equal(t, SourceGoogleScholar, sch.Source())
}

// When an expired profile is refreshed and the listing contains papers that
// are not yet in the article cache, they must be seeded from the listing
// (which carries full details) rather than fetched one by one.
func TestSemanticScholar_RefreshSeedsNewPapersWithoutPerPaperRequests(t *testing.T) {
	sch, client := newSemanticScholar(t)

	// Populate the cache with only the first page (30 of 47 papers).
	articles, err := sch.QueryProfileWithMemoryCache("1792904", 30)
	assert.NoError(t, err)
	assert.Len(t, articles, 30)
	assert.Len(t, client.Requests, 1)

	// Expire the profile so the next call refreshes the listing, this time
	// asking for everything: 17 papers are new to the cache.
	profileResult, _ := sch.profile.Load("1792904")
	profile := profileResult.(Profile)
	profile.LastRetrieved = time.Now().Add(-8 * 24 * time.Hour)
	sch.profile.Store("1792904", profile)

	articles, err = sch.QueryProfileWithMemoryCache("1792904", 100)
	assert.NoError(t, err)
	assert.Len(t, articles, 47)
	assert.Len(t, client.Requests, 3, "expected only the two listing pages, no per-paper requests")
	for _, r := range client.Requests {
		assert.NotContains(t, r.URL.Path, "/graph/v1/paper/")
	}
}

// An unknown source kind is an error and leaves the current source in place,
// rather than silently falling back to scraping Google.
func TestSetSource_UnknownKindIsAnError(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	assert.NoError(t, sch.SetSource(SourceSemanticScholar))
	assert.Equal(t, SourceSemanticScholar, sch.Source())

	err := sch.SetSource(SourceKind("semantic-scholar"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "semantic-scholar")
	assert.Equal(t, SourceSemanticScholar, sch.Source(), "source must be unchanged after a rejected kind")
}
