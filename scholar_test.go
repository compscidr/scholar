package go_scholar

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// MockHTTPClient implements HTTPClient interface for testing
type MockHTTPClient struct{}

// MockRateLimitHTTPClient implements HTTPClient interface for testing rate limiting
type MockRateLimitHTTPClient struct {
	callCount    int
	shouldReturn429 bool
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	
	// Mock profile request - check if it's a profile query
	if strings.Contains(url, "/citations?user=") && strings.Contains(url, "&cstart=") {
		return m.mockProfileResponse()
	}
	
	// Mock article request - check if it's an article view
	if strings.Contains(url, "view_citation") {
		return m.mockArticleResponse()
	}
	
	// Default to empty response for unknown URLs
	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func (m *MockHTTPClient) mockProfileResponse() (*http.Response, error) {
	content, err := os.ReadFile("sample_author_page.html")
	if err != nil {
		return nil, err
	}
	
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(content))),
	}, nil
}

func (m *MockHTTPClient) mockArticleResponse() (*http.Response, error) {
	content, err := os.ReadFile("sample_article_page.html")
	if err != nil {
		return nil, err
	}
	
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(content))),
	}, nil
}

func (m *MockRateLimitHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.callCount++
	
	// Return 429 for the first call to test retry logic
	if m.shouldReturn429 && m.callCount == 1 {
		return &http.Response{
			StatusCode: 429,
			Status:     "Too Many Requests",
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	}
	
	// For subsequent calls or when not testing 429, return success
	url := req.URL.String()
	
	// Mock profile request - check if it's a profile query
	if strings.Contains(url, "/citations?user=") && strings.Contains(url, "&cstart=") {
		return m.mockProfileResponse()
	}
	
	// Mock article request - check if it's an article view
	if strings.Contains(url, "view_citation") {
		return m.mockArticleResponse()
	}
	
	// Default to empty response for unknown URLs
	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func (m *MockRateLimitHTTPClient) mockProfileResponse() (*http.Response, error) {
	content, err := os.ReadFile("sample_author_page.html")
	if err != nil {
		return nil, err
	}
	
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(content))),
	}, nil
}

func (m *MockRateLimitHTTPClient) mockArticleResponse() (*http.Response, error) {
	content, err := os.ReadFile("sample_article_page.html")
	if err != nil {
		return nil, err
	}
	
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(content))),
	}, nil
}

// Test article limiting functionality
func TestArticleLimiting(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	sch.SetHTTPClient(&MockHTTPClient{})
	sch.SetRequestDelay(1 * time.Millisecond) // Fast delay for testing
	
	// Test different limits
	testCases := []int{1, 2, 5, 10}
	
	for _, limit := range testCases {
		t.Run(fmt.Sprintf("Limit_%d", limit), func(t *testing.T) {
			articles, err := sch.QueryProfile("SbUmSEAAAAAJ", limit)
			assert.NoError(t, err)
			assert.Len(t, articles, limit, "Should return exactly %d articles", limit)
			
			// Verify articles have titles (basic sanity check)
			for i, article := range articles {
				assert.NotEmpty(t, article.Title, "Article %d should have a title", i+1)
			}
		})
	}
}

func TestGetArticles(t *testing.T) {
	// Test that we can create a Scholar instance and set mock client
	sch := New("profiles.json", "articles.json")
	// Set a fast delay for testing
	sch.SetRequestDelay(1 * time.Millisecond)
	sch.SetHTTPClient(&MockHTTPClient{})
	
	// Test should not make real network requests
	assert.NotNil(t, sch)
}

func TestScholarQuerier(t *testing.T) {
	// Test basic Scholar creation
	sch := New("profiles.json", "articles.json")
	assert.NotNil(t, sch)
}

func TestMockHTTPClient(t *testing.T) {
	// Test that MockHTTPClient returns appropriate responses
	mock := &MockHTTPClient{}
	
	// Test profile request
	profileReq, _ := http.NewRequest("GET", "https://scholar.google.com/citations?user=SbUmSEAAAAAJ&cstart=0&pagesize=1", nil)
	profileResp, err := mock.Do(profileReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, profileResp.StatusCode)
	
	// Test article request
	articleReq, _ := http.NewRequest("GET", "https://scholar.google.com/citations?view_op=view_citation&hl=en&user=SbUmSEAAAAAJ", nil)
	articleResp, err := mock.Do(articleReq)
	assert.Nil(t, err)
	assert.Equal(t, 200, articleResp.StatusCode)
	
	// Test unknown request
	unknownReq, _ := http.NewRequest("GET", "https://example.com", nil)
	unknownResp, err := mock.Do(unknownReq)
	assert.Nil(t, err)
	assert.Equal(t, 404, unknownResp.StatusCode)
}

func TestProfileQuerier(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	// Set a fast delay for testing to avoid slow tests
	sch.SetRequestDelay(1 * time.Millisecond)
	// Set mock HTTP client to avoid real network requests
	sch.SetHTTPClient(&MockHTTPClient{})
	
	articles, err := sch.QueryProfile("SbUmSEAAAAAJ", 1)
	assert.Nil(t, err)
	assert.NotEmpty(t, articles)

	for _, article := range articles {
		fmt.Println(article)
	}
}

func TestThrottling(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	// Set a very short delay for testing (10ms)
	sch.SetRequestDelay(10 * time.Millisecond)
	sch.SetHTTPClient(&MockHTTPClient{})
	
	// Make multiple requests and measure timing
	start := time.Now()
	
	// Make 3 requests
	for i := 0; i < 3; i++ {
		_, err := sch.QueryProfile("SbUmSEAAAAAJ", 1)
		assert.Nil(t, err)
	}
	
	elapsed := time.Since(start)
	
	// Should take at least 2 * 10ms = 20ms (2 delays between 3 requests)
	// We allow some tolerance for test timing
	assert.True(t, elapsed >= 20*time.Millisecond, "Throttling should enforce delays between requests")
}

func TestRateLimitRetry(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	// Set a very short delay for testing
	sch.SetRequestDelay(1 * time.Millisecond)
	
	mockClient := &MockRateLimitHTTPClient{shouldReturn429: true}
	sch.SetHTTPClient(mockClient)
	
	// This should succeed after the first 429 retry
	// Use queryArticles=false to avoid making article queries which would increase call count
	articles, err := sch.QueryProfileDumpResponse("SbUmSEAAAAAJ", false, 1, false)
	assert.Nil(t, err)
	assert.NotEmpty(t, articles)
	
	// Should have made 2 calls (first 429, second success)
	assert.Equal(t, 2, mockClient.callCount)
}

func TestRequestDelayConfiguration(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	
	// Test default delay (2 seconds)
	assert.Equal(t, 2*time.Second, sch.requestDelay)
	
	// Test setting custom delay
	customDelay := 500 * time.Millisecond
	sch.SetRequestDelay(customDelay)
	assert.Equal(t, customDelay, sch.requestDelay)
}

// MockAlwaysFailHTTPClient returns 500 to simulate server failure without
// triggering the 429 retry/backoff logic (which is tested separately in TestRateLimitRetry).
type MockAlwaysFailHTTPClient struct{}

func (m *MockAlwaysFailHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 500,
		Status:     "Internal Server Error",
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

// Test that when profile cache is expired and refresh fails, stale cached data is returned
func TestStaleCacheFallback(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	sch.SetRequestDelay(1 * time.Millisecond)
	sch.SetHTTPClient(&MockHTTPClient{})

	// First query populates the cache
	articles, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, articles)
	originalCount := len(articles)

	// Expire the profile cache by storing it with an old timestamp
	profileResult, _ := sch.profile.Load("SbUmSEAAAAAJ")
	profile := profileResult.(Profile)
	profile.LastRetrieved = time.Now().Add(-8 * 24 * time.Hour) // 8 days ago (past 7-day expiry)
	sch.profile.Store("SbUmSEAAAAAJ", profile)

	// Now switch to a client that always fails
	sch.SetHTTPClient(&MockAlwaysFailHTTPClient{})

	// Query again — should fall back to stale cache, not return an error
	articles, err = sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.NoError(t, err, "Should not return error when stale cache is available")
	assert.Equal(t, originalCount, len(articles), "Should return same articles from stale cache")
}

// Test that profile refresh with queryArticles=false correctly preserves article URLs
// and returns cached article details
func TestProfileRefreshPreservesArticles(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	sch.SetRequestDelay(1 * time.Millisecond)
	sch.SetHTTPClient(&MockHTTPClient{})

	// First query populates both profile and article caches (queryArticles=true for cache miss)
	articles, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, articles)
	originalCount := len(articles)

	// Verify articles have full details (authors populated by QueryArticle)
	for _, a := range articles {
		assert.NotEmpty(t, a.ScholarURL, "Article should have ScholarURL")
		assert.NotEmpty(t, a.Authors, "Article should have Authors from detail page")
	}

	// Expire the profile cache so the next call triggers a profile-only refresh
	profileResult, _ := sch.profile.Load("SbUmSEAAAAAJ")
	profile := profileResult.(Profile)
	profile.LastRetrieved = time.Now().Add(-8 * 24 * time.Hour) // 8 days ago
	sch.profile.Store("SbUmSEAAAAAJ", profile)

	// Second query should refresh profile (queryArticles=false) and serve article details from cache
	articles2, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.NoError(t, err)
	assert.Equal(t, originalCount, len(articles2), "Should return same number of articles after profile refresh")

	// Verify articles still have full details from cache
	for _, a := range articles2 {
		assert.NotEmpty(t, a.ScholarURL, "Article should still have ScholarURL after profile refresh")
		assert.NotEmpty(t, a.Authors, "Article should still have Authors from cache after profile refresh")
	}
}

// Test pagination behavior by attempting to request more articles than available on one page
func TestPaginationLogic(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	sch.SetRequestDelay(1 * time.Millisecond)
	sch.SetHTTPClient(&MockHTTPClient{})
	
	// The sample data has 58 articles in one page. When we request more, 
	// pagination should kick in but since mock returns the same page, we should get 58
	articles, err := sch.QueryProfileDumpResponse("SbUmSEAAAAAJ", false, 100, false)
	assert.NoError(t, err)
	
	// Should return 58 articles (all available in sample data)
	assert.Equal(t, 58, len(articles), "Should return all 58 articles from sample data")
	
	// Verify articles have titles (basic sanity check)
	for i, article := range articles {
		assert.NotEmpty(t, article.Title, "Article %d should have a title", i+1)
	}
}

// MockBlockedHTTPClient mimics Google Scholar refusing a datacenter IP: every
// request gets a 403 with the "automated queries" block page. It counts calls.
type MockBlockedHTTPClient struct{ Calls int }

func (m *MockBlockedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.Calls++
	return &http.Response{
		StatusCode: 403,
		Status:     "403 Forbidden",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("<html><title>Sorry...</title><body>We're sorry... but your computer or network may be sending automated queries.</body></html>")),
	}, nil
}

// MockNotFoundHTTPClient returns 404 for everything, to contrast with a block.
type MockNotFoundHTTPClient struct{}

func (m *MockNotFoundHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 404,
		Status:     "404 Not Found",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

// A cache miss that fails must not be retried on every call: with no cached
// data to fall back to, each call would otherwise be a new request to Google,
// which only reinforces an "automated queries" block.
func TestCacheMissFailureCooldown(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	sch.SetRequestDelay(1 * time.Millisecond)
	client := &MockBlockedHTTPClient{}
	sch.SetHTTPClient(client)

	_, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.Error(t, err)
	assert.Equal(t, 1, client.Calls)

	for i := 0; i < 5; i++ {
		_, err = sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
		assert.Error(t, err, "calls during the cooldown must still report the failure")
	}
	assert.Equal(t, 1, client.Calls, "no further requests during the cooldown")
	assert.True(t, errors.Is(err, ErrBlocked), "the cooldown error must preserve the original cause: %v", err)

	// A different user is not affected by this user's failure.
	_, _ = sch.QueryProfileWithMemoryCache("OtherUser", 10)
	assert.Equal(t, 2, client.Calls)

	// Once the cooldown has passed, the next call tries again, and the
	// expired entry is dropped (the retry below fails again, so a fresh
	// entry replaces it; check the timestamp moved).
	sch.failureMu.Lock()
	f := sch.failures["SbUmSEAAAAAJ"]
	expiredAt := time.Now().Add(-sch.failureCooldown - time.Minute)
	f.at = expiredAt
	sch.failures["SbUmSEAAAAAJ"] = f
	sch.failureMu.Unlock()
	_, _ = sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.Equal(t, 3, client.Calls, "expected a retry after the cooldown")
	sch.failureMu.Lock()
	assert.True(t, sch.failures["SbUmSEAAAAAJ"].at.After(expiredAt), "expired entry should have been replaced")
	sch.failureMu.Unlock()

	// Expired entries are removed when noticed, even without a new failure.
	sch.failureMu.Lock()
	sch.failures["OtherUser"] = fetchFailure{at: expiredAt, err: errors.New("old")}
	sch.failureMu.Unlock()
	_, inCooldown := sch.inFailureCooldown("OtherUser")
	assert.False(t, inCooldown)
	sch.failureMu.Lock()
	_, present := sch.failures["OtherUser"]
	sch.failureMu.Unlock()
	assert.False(t, present, "expired entries must be deleted, not left to accumulate")
}

// A successful fetch clears any recorded failure, and the cooldown is configurable.
func TestFailureCooldownClearedOnSuccessAndConfigurable(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	sch.SetRequestDelay(1 * time.Millisecond)
	sch.SetHTTPClient(&MockBlockedHTTPClient{})

	_, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.Error(t, err)

	// Disabling the cooldown means the very next call goes to the network again.
	sch.SetFailureCooldown(0)
	sch.SetHTTPClient(&MockHTTPClient{})
	articles, err := sch.QueryProfileWithMemoryCache("SbUmSEAAAAAJ", 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, articles)

	sch.failureMu.Lock()
	_, stillRecorded := sch.failures["SbUmSEAAAAAJ"]
	sch.failureMu.Unlock()
	assert.False(t, stillRecorded, "a successful fetch must clear the recorded failure")

	// With the cooldown disabled, failures are not recorded at all.
	sch.SetHTTPClient(&MockBlockedHTTPClient{})
	_, err = sch.QueryProfileWithMemoryCache("AnotherUser", 10)
	assert.Error(t, err)
	sch.failureMu.Lock()
	_, recorded := sch.failures["AnotherUser"]
	sch.failureMu.Unlock()
	assert.False(t, recorded, "nothing should be recorded while the cooldown is disabled")
}

// Google's block page is reported as ErrBlocked so callers can tell it apart
// from a profile that doesn't exist.
func TestErrBlocked(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	sch.SetRequestDelay(1 * time.Millisecond)

	sch.SetHTTPClient(&MockBlockedHTTPClient{})
	_, err := sch.QueryProfile("SbUmSEAAAAAJ", 10)
	assert.True(t, errors.Is(err, ErrBlocked), "expected ErrBlocked, got %v", err)
	assert.Contains(t, err.Error(), "403", "the status should still be in the message")

	sch.SetFailureCooldown(0)
	sch.SetHTTPClient(&MockNotFoundHTTPClient{})
	_, err = sch.QueryProfile("SbUmSEAAAAAJ", 10)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrBlocked), "a plain 404 is not a block")
}
