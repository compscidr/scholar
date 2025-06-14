package go_scholar

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTPClient interface to allow mocking of HTTP requests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

const BaseURL = "https://scholar.google.com"
const AGENT = "Mozilla/5.0 (X11; Linux x86_64; rv:60.0) Gecko/20100101 Firefox/81.0"
const MAX_TIME_PROFILE = time.Second * 3600 * 24     // 1 Day
const MAX_TIME_ARTICLE = time.Second * 3600 * 24 * 7 // 1 week

type Article struct {
	Title               string
	Authors             string
	ScholarURL          string
	Year                int
	Month               int
	Day                 int
	NumCitations        int
	Articles            int // if there are more than one article within this publication (it will also tell how big the arrays below are)
	Description         string
	PdfURL              string
	Journal             string
	Volume              string
	Pages               string
	Publisher           string
	ScholarCitedByURLs  []string
	ScholarVersionsURLs []string
	ScholarRelatedURLs  []string
	LastRetrieved       time.Time
}

type Profile struct {
	User          string
	LastRetrieved time.Time
	Articles      []string // list of article URLs - we'd still need to look them up in the article map
}

type Scholar struct {
	articles      sync.Map      // map of articles by URL
	profile       sync.Map      // map of profile by User string
	httpClient    HTTPClient    // HTTP client for making requests
	rateLimiter   *time.Ticker  // rate limiter for throttling requests
	requestDelay  time.Duration // delay between requests
	lastRequest   time.Time     // timestamp of last request
	requestMutex  sync.Mutex    // mutex to synchronize requests
}

func New(profileCache string, articleCache string) *Scholar {
	// Initialize the base Scholar struct with default HTTP client and rate limiter
	// Default to 2 seconds between requests to be conservative with Google Scholar's rate limits
	requestDelay := 2 * time.Second
	sch := Scholar{
		httpClient:   &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{},
			},
		},
		requestDelay: requestDelay,
		lastRequest:  time.Time{}, // zero time initially
	}

	profileFile, err := os.Open(profileCache)
	if err != nil {
		println("Error opening profile cache file: " + profileCache + " - creating new cache")
		return &sch
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			println("Error closing profile cache file: " + profileCache)
		}
	}(profileFile)
	profileDecoder := json.NewDecoder(profileFile)
	var regularProfileMap map[string]Profile
	err = profileDecoder.Decode(&regularProfileMap)
	if err != nil {
		println("Error decoding profile file: " + profileCache + " - creating new cache")
		return &sch
	}

	articleFile, err := os.Open(articleCache)
	if err != nil {
		println("Error opening article cache file: " + articleCache + " - creating new cache")
		return &sch
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			println("Error closing article cache file: " + articleCache)
		}
	}(articleFile)
	articleDecoder := json.NewDecoder(articleFile)
	var regularArticleMap map[string]*Article
	err = articleDecoder.Decode(&regularArticleMap)
	if err != nil {
		println("Error decoding article cache file: " + articleCache + " - creating new cache")
		return &sch
	}

	// convert the regular maps to sync maps
	for key, value := range regularProfileMap {
		sch.profile.Store(key, value)
	}
	fmt.Printf("Loaded cache into memory with %d profiles\n", len(regularProfileMap))
	for key, value := range regularArticleMap {
		sch.articles.Store(key, value)
	}
	fmt.Printf("Loaded cache into memory with %d articles\n", len(regularArticleMap))

	return &sch
}

// SetHTTPClient allows setting a custom HTTP client (useful for testing)
func (sch *Scholar) SetHTTPClient(client HTTPClient) {
	sch.httpClient = client
}

// SetRequestDelay allows setting a custom delay between requests for throttling
func (sch *Scholar) SetRequestDelay(delay time.Duration) {
	sch.requestDelay = delay
}

// makeThrottledRequest makes an HTTP request with rate limiting and retry logic for 429 errors
func (sch *Scholar) makeThrottledRequest(req *http.Request) (*http.Response, error) {
	const maxRetries = 3
	const baseBackoffDelay = 5 * time.Second
	
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Apply rate limiting
		sch.requestMutex.Lock()
		if !sch.lastRequest.IsZero() {
			elapsed := time.Since(sch.lastRequest)
			if elapsed < sch.requestDelay {
				sleepTime := sch.requestDelay - elapsed
				sch.requestMutex.Unlock()
				time.Sleep(sleepTime)
				sch.requestMutex.Lock()
			}
		}
		sch.lastRequest = time.Now()
		sch.requestMutex.Unlock()
		
		// Make the request
		resp, err := sch.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		
		// If not a rate limit error, return the response
		if resp.StatusCode != 429 {
			return resp, nil
		}
		
		// Handle 429 (Too Many Requests) with exponential backoff
		resp.Body.Close() // Close the response body before retrying
		
		if attempt == maxRetries {
			return nil, fmt.Errorf("max retries (%d) exceeded due to rate limiting (HTTP 429)", maxRetries)
		}
		
		// Exponential backoff: baseDelay * 2^attempt
		backoffDelay := baseBackoffDelay * time.Duration(1<<uint(attempt))
		fmt.Printf("Rate limited (429), retrying in %v (attempt %d/%d)\n", backoffDelay, attempt+1, maxRetries)
		time.Sleep(backoffDelay)
	}
	
	return nil, fmt.Errorf("unexpected error in retry logic")
}

func (sch *Scholar) SaveCache(profileCache string, articleCache string) {
	profileFile, err := os.Create(profileCache)
	if err != nil {
		println("Error opening profile cache file: " + profileCache)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			println("Error closing profile cache file: " + profileCache)
		}
	}(profileFile)
	profileEncoder := json.NewEncoder(profileFile)
	regularProfileMap := make(map[string]interface{})
	sch.profile.Range(func(key, value interface{}) bool {
		regularProfileMap[key.(string)] = value
		return true
	})
	err = profileEncoder.Encode(regularProfileMap)
	if err != nil {
		println("Error encoding profile cache file: " + profileCache)
	}

	articleFile, err := os.Create(articleCache)
	if err != nil {
		println("Error opening article cache file: " + articleCache)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			println("Error closing profile cache file: " + articleCache)
		}
	}(articleFile)
	articleEncoder := json.NewEncoder(articleFile)
	regularArticleMap := make(map[string]interface{})
	sch.articles.Range(func(key, value interface{}) bool {
		regularArticleMap[key.(string)] = value
		return true
	})
	err = articleEncoder.Encode(regularArticleMap)
	if err != nil {
		println("Error encoding cache file: " + articleCache)
	}
	if err == nil {
		println("Saved cache")
	}
}

func (a Article) String() string {
	return "Article(\n  Title=" + a.Title + "\n  authors=" + a.Authors + "\n  ScholarURL=" + a.ScholarURL + "\n  Year=" + strconv.Itoa(a.Year) + "\n  Month=" + strconv.Itoa(a.Month) + "\n  Day=" + strconv.Itoa(a.Day) + "\n  NumCitations=" + strconv.Itoa(a.NumCitations) + "\n  Articles=" + strconv.Itoa(a.Articles) + "\n  Description=" + a.Description + "\n  PdfURL=" + a.PdfURL + "\n  Journal=" + a.Journal + "\n  Volume=" + a.Volume + "\n  Pages=" + a.Pages + "\n  Publisher=" + a.Publisher + "\n  scholarCitedByURL=" + strings.Join(a.ScholarCitedByURLs, ", ") + "\n  scholarVersionsURL=" + strings.Join(a.ScholarVersionsURLs, ", ") + "\n  scholarRelatedURL=" + strings.Join(a.ScholarRelatedURLs, ", ") + "\n  LastRetrieved=" + a.LastRetrieved.String() + "\n)"
}

func (sch *Scholar) QueryProfile(user string, limit int) ([]*Article, error) {
	return sch.QueryProfileDumpResponse(user, true, limit, false)
}

func (sch *Scholar) QueryProfileWithMemoryCache(user string, limit int) ([]*Article, error) {

	profileResult, profileOk := sch.profile.Load(user)
	if profileOk {
		profile := profileResult.(Profile)
		lastAccess := profile.LastRetrieved
		if (time.Now().Sub(lastAccess)).Seconds() > MAX_TIME_PROFILE.Seconds() {
			println("Profile cache expired for User: " + user)
			articles, err := sch.QueryProfileDumpResponse(user, true, limit, false)
			if err == nil {
				var articleList []string
				for _, article := range articles {
					articleList = append(articleList, article.ScholarURL)
				}
				newProfile := Profile{User: user, LastRetrieved: time.Now(), Articles: articleList}
				sch.profile.Delete(user)
				sch.profile.Store(user, newProfile)
			} else {
				return nil, err
			}
		} else {
			println("Profile cache hit for User: " + user)
			// cache hit, return the Articles
			articles := make([]*Article, 0)
			for _, articleURL := range profile.Articles {
				articleResult, articleOk := sch.articles.Load(articleURL)
				if articleOk {
					cacheArticle := articleResult.(*Article)
					if (time.Now().Sub(cacheArticle.LastRetrieved)).Seconds() > MAX_TIME_ARTICLE.Seconds() {
						println("Cache expired for article: " + articleURL + "\nLast Retrieved: " + cacheArticle.LastRetrieved.String() + "\nDifference: " + time.Now().Sub(cacheArticle.LastRetrieved).String())
						article, err := sch.QueryArticle(articleURL, &Article{}, false)
						if err == nil {
							sch.articles.Store(articleURL, article)
							articles = append(articles, article)
						}
					} else {
						println("Cache hit for article: " + articleURL)
						articles = append(articles, cacheArticle)
					}
				} else {
					// cache miss, query the article
					println("Cache miss for article: " + articleURL)
					article, err := sch.QueryArticle(articleURL, &Article{}, false)
					if err == nil {
						articles = append(articles, article)
						sch.articles.Store(articleURL, article)
					}
				}
			}
			return articles, nil
		}
	} else {
		println("Profile cache miss for User: " + user)
		articles, err := sch.QueryProfileDumpResponse(user, true, limit, false)
		if err == nil {
			var articleList []string
			for _, article := range articles {
				articleList = append(articleList, article.ScholarURL)
			}
			newProfile := Profile{User: user, LastRetrieved: time.Now(), Articles: articleList}
			sch.profile.Store(user, newProfile)
			return articles, nil
		} else {
			return nil, err
		}
	}
	return nil, errors.New("Shouldn't have got here")
}

// QueryProfileDumpResponse queries the profile of a User and returns a list of Articles
// if queryArticles is true, it will also query the Articles for extra information which isn't present on the profile page
//
//	we may wish to set this to false if we are only interested in some article info, or we have a cache hit and we just
//	want to get updated information from the profile page only to save requests
//
// if dumpResponse is true, it will print the response to stdout (useful for debugging)
func (sch *Scholar) QueryProfileDumpResponse(user string, queryArticles bool, limit int, dumpResponse bool) ([]*Article, error) {
	var articles []*Article
	
	// Use a reasonable page size for each request, but not too large to avoid timeouts
	// Google Scholar typically works with pagesize 20-100
	pageSize := 80
	if limit < pageSize {
		pageSize = limit
	}
	if pageSize < 20 {
		pageSize = 20 // Google Scholar typically has a minimum page size
	}
	
	cstart := 0
	remainingArticles := limit
	
	for remainingArticles > 0 {
		// Fetch a page of articles
		pageArticles, err := sch.fetchProfilePage(user, cstart, pageSize, queryArticles, dumpResponse)
		if err != nil {
			return nil, err
		}
		
		// If no articles returned, we've reached the end
		if len(pageArticles) == 0 {
			break
		}
		
		// Add articles up to our limit
		articlesToAdd := remainingArticles
		if len(pageArticles) < articlesToAdd {
			articlesToAdd = len(pageArticles)
		}
		
		articles = append(articles, pageArticles[:articlesToAdd]...)
		remainingArticles -= articlesToAdd
		
		// If we got fewer articles than requested pagesize, we've reached the end
		if len(pageArticles) < pageSize {
			break
		}
		
		// Move to next page
		cstart += pageSize
	}

	return articles, nil
}

// fetchProfilePage fetches a single page of articles from Google Scholar
func (sch *Scholar) fetchProfilePage(user string, cstart, pageSize int, queryArticles bool, dumpResponse bool) ([]*Article, error) {
	var articles []*Article
	
	requestURL := BaseURL + "/citations?user=" + user + "&cstart=" + strconv.Itoa(cstart) + "&pagesize=" + strconv.Itoa(pageSize)
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", AGENT)
	
	resp, err := sch.makeThrottledRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		rateLimitRemaining := resp.Header.Get("x-ratelimit-remaining")
		errorString := fmt.Sprintf("Scholar: HTTP Status Code from URL: %s %d %s rate limit remaining?: %s", requestURL, resp.StatusCode, resp.Status, rateLimitRemaining)
		return nil, errors.New(errorString)
	}

	if dumpResponse {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		// Reset body for subsequent parsing
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		println("GOT AUTHOR PAGE (cstart=" + strconv.Itoa(cstart) + "): \n", string(bodyBytes))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	// Process articles from this page
	doc.Find(".gsc_a_tr").Each(func(i int, s *goquery.Selection) {
		article := &Article{}
		entry := s.Find(".gsc_a_t")
		link := entry.Find(".gsc_a_at")
		article.Title = link.Text()

		tempURL, _ := link.Attr("href")
		article.Year, _ = strconv.Atoi(s.Find(".gsc_a_y").Find("span").Text())
		article.NumCitations, _ = strconv.Atoi(s.Find(".gsc_a_c").Children().First().Text())

		if queryArticles {
			articleResult, articleOk := sch.articles.Load(BaseURL + tempURL)
			if articleOk {
				// hit the cache
				cacheArticle := articleResult.(*Article)
				if (time.Now().Sub(article.LastRetrieved)).Seconds() > MAX_TIME_ARTICLE.Seconds() {
					println("Cache expired for article" + BaseURL + tempURL + "\nLast Retrieved: " + cacheArticle.LastRetrieved.String() + "\nDifference: " + time.Now().Sub(cacheArticle.LastRetrieved).String())
					// expired cache entry, replace it
					article, err = sch.QueryArticle(BaseURL+tempURL, article, dumpResponse)
					if err == nil {
						// only delete and store if we were successful
						sch.articles.Delete(BaseURL + tempURL)
						sch.articles.Store(BaseURL+tempURL, article)
					}
				} else {
					println("Cache hit for article" + BaseURL + tempURL)
					// not expired, update any new information
					cacheArticle.NumCitations = article.NumCitations // update the citations since thats all that might change
					article = cacheArticle
					sch.articles.Store(BaseURL+tempURL, article)
				}
			} else {
				println("Cache miss for article" + BaseURL + tempURL)
				article, err = sch.QueryArticle(BaseURL+tempURL, article, dumpResponse)
				if err == nil {
					sch.articles.Store(BaseURL+tempURL, article)
				}
			}
		}
		articles = append(articles, article)
	})

	return articles, nil
}

func (sch *Scholar) QueryArticle(url string, article *Article, dumpResponse bool) (*Article, error) {
	article.ScholarURL = url
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", AGENT)
	
	resp, err := sch.makeThrottledRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		errorString := fmt.Sprintf("Scholar: HTTP Status Code: %d", resp.StatusCode)
		return nil, errors.New(errorString)
	}

	if dumpResponse {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		println("GOT ARTICLE: \n", string(bodyBytes))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	article.LastRetrieved = time.Now()
	article.Articles = 0
	article.PdfURL, _ = doc.Find(".gsc_oci_title_ggi").Children().First().Attr("href") // assume the link is the first child
	doc.Find(".gs_scl").Each(func(i int, s *goquery.Selection) {
		text := s.Find(".gsc_oci_field").Text()
		if text == "Authors" {
			article.Authors = s.Find(".gsc_oci_value").Text()
		}
		if text == "Publication date" {
			datestring := s.Find(".gsc_oci_value").Text()
			parts := strings.Split(datestring, "/")
			if len(parts) == 3 {
				article.Year, _ = strconv.Atoi(parts[0])
				article.Month, _ = strconv.Atoi(parts[1])
				article.Day, _ = strconv.Atoi(parts[2])
			}
		}
		if text == "Journal" {
			article.Journal = s.Find(".gsc_oci_value").Text()
		}
		if text == "Volume" {
			article.Volume = s.Find(".gsc_oci_value").Text()
		}
		if text == "Pages" {
			article.Pages = s.Find(".gsc_oci_value").Text()
		}
		if text == "Publisher" {
			article.Publisher = s.Find(".gsc_oci_value").Text()
		}
		if text == "Description" {
			article.Description = s.Find(".gsc_oci_value").Text()
		}
		// don't need to parse here, already have it
		//if text == "Total citations" {
		//	citationString := s.Find(".gsc_oci_value").Text()
		//	parts := strings.Split(citationString, "Cited by ")
		//	if len(parts) == 2 {
		//		article.NumCitations, _ = strconv.Atoi(parts[1])
		//	}
		//}
		if text == "Scholar Articles" {
			article.Articles += 1
			articles := s.Find(".gsc_oci_value")
			articles.Find(".gsc_oci_merged_snippet").Each(func(i int, s *goquery.Selection) {
				// each one of these is an article. For a scholar-example with multiple see:
				// https://scholar.google.com/citations?view_op=view_citation&hl=en&user=ECQMeb0AAAAJ&citation_for_view=ECQMeb0AAAAJ:u5HHmVD_uO8C
				// this seems to happen if the entry is a book and there are Articles within it
				s.Find(".gsc_oms_link").Each(func(i int, l *goquery.Selection) {
					linkText := l.Text()
					linkUrl, _ := l.Attr("href")
					if strings.Contains(linkText, "Cited by") {
						article.ScholarCitedByURLs = append(article.ScholarCitedByURLs, linkUrl)
					}
					if strings.Contains(linkText, "Related Articles") {
						article.ScholarRelatedURLs = append(article.ScholarRelatedURLs, linkUrl)
					}
					if strings.Contains(linkText, "versions") {
						article.ScholarVersionsURLs = append(article.ScholarVersionsURLs, linkUrl)
					}
				})
			})
		}
	})
	return article, nil
}
