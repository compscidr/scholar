package go_scholar

import (
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
