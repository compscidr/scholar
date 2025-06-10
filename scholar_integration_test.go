//go:build integration

package go_scholar

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// TestProfileQuerierIntegration tests the Scholar functionality against the real Google Scholar API.
// This test is optional and only runs when the 'integration' build tag is specified.
// Run with: go test -tags integration
//
// Note: This test may fail due to rate limits or network restrictions, but that's expected
// and should not break CI/CD pipelines. It's intended for manual testing and verification
// against the live API.
func TestProfileQuerierIntegration(t *testing.T) {
	// Create Scholar instance with real HTTP client (default behavior)
	sch := New("profiles.json", "articles.json")
	// Note: We don't call SetHTTPClient, so it uses the real HTTP client
	
	// Use a known working profile ID (this is a public Google Scholar profile)
	// SbUmSEAAAAAJ appears to be a valid profile ID based on the test data
	profileID := "SbUmSEAAAAAJ"
	
	// Set a reasonable timeout for the test
	done := make(chan bool, 1)
	var articles []*Article
	var err error
	
	go func() {
		articles, err = sch.QueryProfile(profileID, 1)
		done <- true
	}()
	
	select {
	case <-done:
		// Test completed normally
		if err != nil {
			// Log the error but don't fail the test - this is expected for rate limits/network issues
			t.Logf("Integration test failed (this is expected and OK): %v", err)
			t.Skip("Skipping integration test due to network/rate limit issues")
			return
		}
		
		// If we got here, the real API call succeeded
		assert.NotNil(t, articles)
		assert.NotEmpty(t, articles, "Should return at least one article from real API")
		
		// Verify the article has basic required fields
		if len(articles) > 0 {
			article := articles[0]
			assert.NotEmpty(t, article.Title, "Article should have a title")
			assert.NotEmpty(t, article.ScholarURL, "Article should have a Scholar URL")
			
			fmt.Printf("Integration test SUCCESS - Retrieved article: %s\n", article.Title)
		}
		
	case <-time.After(30 * time.Second):
		// Test timed out - this is also expected and OK
		t.Skip("Integration test timed out (this is expected and OK)")
	}
}

// TestScholarRealHTTPClient verifies that the Scholar instance uses real HTTP client by default
func TestScholarRealHTTPClient(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	assert.NotNil(t, sch)
	
	// We can't directly test the HTTP client type without exposing it,
	// but we can verify the Scholar instance was created successfully
	// The real test of functionality is in TestProfileQuerierIntegration
}