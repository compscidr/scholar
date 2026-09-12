package go_scholar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SemanticScholarBaseURL is the Academic Graph API endpoint.
const SemanticScholarBaseURL = "https://api.semanticscholar.org/graph/v1"

// semanticScholarFields is the field list requested for papers; everything
// mapped onto Article comes from here.
const semanticScholarFields = "title,authors,year,publicationDate,venue,journal,citationCount,externalIds,url,abstract,openAccessPdf"

// semanticScholarPageSize is the API's maximum page size for author papers.
const semanticScholarPageSize = 100

// semanticScholarSource implements source against the Semantic Scholar API.
type semanticScholarSource struct{ sch *Scholar }

// s2Paper mirrors the subset of the API's paper object we use.
type s2Paper struct {
	PaperID         string `json:"paperId"`
	URL             string `json:"url"`
	Title           string `json:"title"`
	Abstract        string `json:"abstract"`
	Venue           string `json:"venue"`
	Year            int    `json:"year"`
	PublicationDate string `json:"publicationDate"`
	CitationCount   int    `json:"citationCount"`
	Authors         []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Journal *struct {
		Name   string `json:"name"`
		Volume string `json:"volume"`
		Pages  string `json:"pages"`
	} `json:"journal"`
	OpenAccessPdf *struct {
		URL    string `json:"url"`
		Status string `json:"status"`
	} `json:"openAccessPdf"`
}

type s2PapersPage struct {
	Offset int       `json:"offset"`
	Next   *int      `json:"next"`
	Data   []s2Paper `json:"data"`
}

// Every listing page is requested with the full field set.
func (s semanticScholarSource) listingIsComplete() bool { return true }

// fetchProfile pages through /author/{id}/papers until limit or the end.
// Each page already carries full details, so details is ignored.
func (s semanticScholarSource) fetchProfile(user string, limit int, details bool) ([]*Article, error) {
	var articles []*Article
	offset := 0
	for len(articles) < limit {
		pageSize := semanticScholarPageSize
		if remaining := limit - len(articles); remaining < pageSize {
			pageSize = remaining
		}
		endpoint := fmt.Sprintf("%s/author/%s/papers?fields=%s&limit=%d&offset=%d",
			SemanticScholarBaseURL, url.PathEscape(user), semanticScholarFields, pageSize, offset)
		var page s2PapersPage
		if err := s.get(endpoint, &page); err != nil {
			return nil, err
		}
		for _, p := range page.Data {
			if len(articles) == limit {
				break
			}
			articles = append(articles, p.toArticle())
		}
		if page.Next == nil || len(page.Data) == 0 {
			break
		}
		offset = *page.Next
	}
	return articles, nil
}

// fetchArticle refreshes one paper by its Semantic Scholar URL.
func (s semanticScholarSource) fetchArticle(key string) (*Article, error) {
	paperID := key[strings.LastIndex(key, "/")+1:]
	if paperID == "" {
		return nil, fmt.Errorf("semantic scholar: cannot derive paper id from %q", key)
	}
	endpoint := fmt.Sprintf("%s/paper/%s?fields=%s", SemanticScholarBaseURL, url.PathEscape(paperID), semanticScholarFields)
	var p s2Paper
	if err := s.get(endpoint, &p); err != nil {
		return nil, err
	}
	a := p.toArticle()
	a.ScholarURL = key // keep the cache key stable even if the API's url differs
	return a, nil
}

// get performs a throttled GET and decodes the JSON body into v.
func (s semanticScholarSource) get(endpoint string, v interface{}) error {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", AGENT)
	if s.sch.apiKey != "" {
		req.Header.Set("x-api-key", s.sch.apiKey)
	}
	resp, err := s.sch.makeThrottledRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return errors.New(fmt.Sprintf("Semantic Scholar: HTTP Status Code from URL: %s %d %s: %s", endpoint, resp.StatusCode, resp.Status, strings.TrimSpace(string(body))))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// toArticle maps an API paper onto the Article shape shared with the Google
// Scholar source, so caches and consumers are source-agnostic.
func (p s2Paper) toArticle() *Article {
	a := &Article{
		Title:         p.Title,
		ScholarURL:    p.URL,
		Year:          p.Year,
		NumCitations:  p.CitationCount,
		Description:   p.Abstract,
		Journal:       p.Venue,
		Articles:      1,
		LastRetrieved: time.Now(),
	}
	if a.ScholarURL == "" {
		a.ScholarURL = "https://www.semanticscholar.org/paper/" + p.PaperID
	}
	names := make([]string, 0, len(p.Authors))
	for _, au := range p.Authors {
		names = append(names, au.Name)
	}
	a.Authors = strings.Join(names, ", ")
	if p.Journal != nil {
		if p.Journal.Name != "" {
			a.Journal = p.Journal.Name
		}
		a.Volume = p.Journal.Volume
		a.Pages = p.Journal.Pages
	}
	if p.OpenAccessPdf != nil && p.OpenAccessPdf.URL != "" {
		a.PdfURL = p.OpenAccessPdf.URL
	}
	// publicationDate is YYYY-MM-DD when known; otherwise only the year is.
	if parts := strings.Split(p.PublicationDate, "-"); len(parts) == 3 {
		if y, err := strconv.Atoi(parts[0]); err == nil {
			a.Year = y
		}
		if m, err := strconv.Atoi(parts[1]); err == nil {
			a.Month = m
		}
		if d, err := strconv.Atoi(parts[2]); err == nil {
			a.Day = d
		}
	}
	return a
}
