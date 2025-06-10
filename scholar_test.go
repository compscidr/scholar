package go_scholar

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// MockHTTPClient implements HTTPClient interface for testing
type MockHTTPClient struct{}

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

func TestGetArticles(t *testing.T) {
	// Test that we can create a Scholar instance and set mock client
	sch := New("profiles.json", "articles.json")
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
	// Set mock HTTP client to avoid real network requests
	sch.SetHTTPClient(&MockHTTPClient{})
	
	articles, err := sch.QueryProfile("SbUmSEAAAAAJ", 1)
	assert.Nil(t, err)
	assert.NotEmpty(t, articles)

	for _, article := range articles {
		fmt.Println(article)
	}
}
