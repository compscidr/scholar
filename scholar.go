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
const MAX_TIME_PROFILE = time.Second * 3600 * 24 * 7  // 1 week
const MAX_TIME_ARTICLE = time.Second * 3600 * 24 * 30 // 30 days

// DefaultFailureCooldown is how long, after a failed fetch for a user with no
// cached data, the library refuses to contact Google again for that user.
// Google blocks IPs it thinks send automated queries; retrying on every call
// only reinforces that. Configurable via SetFailureCooldown.
const DefaultFailureCooldown = time.Hour

// ErrBlocked is wrapped into the error returned when Google Scholar answers
// with its "automated queries" block page rather than the requested profile.
// Check with errors.Is to tell a blocked IP apart from a missing profile.
var ErrBlocked = errors.New("blocked by Google Scholar (automated queries)")

// SourceKind selects where publication data comes from.
type SourceKind string

const (
	// SourceGoogleScholar scrapes scholar.google.com profile and article pages
	// (the original behaviour). Google refuses many datacenter IPs; see ErrBlocked.
	SourceGoogleScholar SourceKind = "google_scholar"
	// SourceSemanticScholar uses the Semantic Scholar Academic Graph API. Coverage
	// and citation counts differ from Google Scholar, but it is a real API with
	// no IP blocking. Users are identified by their Semantic Scholar author id.
	SourceSemanticScholar SourceKind = "semantic_scholar"
)

// source is the seam between the cache layer and a publication backend.
type source interface {
	// fetchProfile returns up to limit articles for user, newest listing order
	// as the backend provides it. When details is false the backend may skip
	// per-article lookups and return only what the listing provides.
	fetchProfile(user string, limit int, details bool) ([]*Article, error)
	// fetchArticle returns full details for the article identified by key
	// (the value stored in Article.ScholarURL, which the article cache is
	// keyed by).
	fetchArticle(key string) (*Article, error)
	// listingIsComplete reports whether fetchProfile's articles carry full
	// details even when details is false. When true, the cache layer can
	// seed new articles straight from a listing instead of fetching each.
	listingIsComplete() bool
}

// googleSource adapts the existing scraper to the source interface.
type googleSource struct{ sch *Scholar }

// The profile page only lists title, authors, year and citations; details
// need a per-article fetch.
func (g googleSource) listingIsComplete() bool { return false }

func (g googleSource) fetchProfile(user string, limit int, details bool) ([]*Article, error) {
	return g.sch.QueryProfileDumpResponse(user, details, limit, false)
}

func (g googleSource) fetchArticle(key string) (*Article, error) {
	return g.sch.QueryArticle(key, &Article{}, false)
}

// fetchFailure records the last failed fetch for a user.
type fetchFailure struct {
	at  time.Time
	err error
}

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

	sourceKind SourceKind // which backend the cache layer talks to
	src        source
	apiKey     string // optional Semantic Scholar API key

	failureCooldown time.Duration           // see DefaultFailureCooldown
	failures        map[string]fetchFailure // last failed fetch per user, cleared on success
	failureMu       sync.Mutex
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

		failureCooldown: DefaultFailureCooldown,
		failures:        make(map[string]fetchFailure),
	}
	sch.SetSource(SourceGoogleScholar)

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

// SetSource selects the publication backend. The default is Google Scholar.
// Cached data is keyed by user id and article URL, so switching sources for
// the same cache files simply results in cache misses for the new ids.
func (sch *Scholar) SetSource(kind SourceKind) {
	sch.sourceKind = kind
	switch kind {
	case SourceSemanticScholar:
		sch.src = semanticScholarSource{sch: sch}
	default:
		sch.sourceKind = SourceGoogleScholar
		sch.src = googleSource{sch: sch}
	}
}

// Source reports the active publication backend.
func (sch *Scholar) Source() SourceKind {
	return sch.sourceKind
}

// SetAPIKey sets the Semantic Scholar API key sent as x-api-key. Optional;
// without it requests share the unauthenticated rate-limit pool.
func (sch *Scholar) SetAPIKey(key string) {
	sch.apiKey = key
}

// SetFailureCooldown sets how long a failed fetch for a user without cached
// data suppresses further requests for that user. Zero disables the cooldown.
func (sch *Scholar) SetFailureCooldown(d time.Duration) {
	sch.failureMu.Lock()
	defer sch.failureMu.Unlock()
	sch.failureCooldown = d
	if d <= 0 {
		sch.failures = make(map[string]fetchFailure)
	}
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
	return sch.src.fetchProfile(user, limit, true)
}

// loadCachedArticles returns articles from the article cache for a given profile.
// Articles that fail to refresh (e.g. due to throttling) are served stale.
func (sch *Scholar) loadCachedArticles(profile Profile) []*Article {
	articles := make([]*Article, 0)
	for _, articleURL := range profile.Articles {
		articleResult, articleOk := sch.articles.Load(articleURL)
		if articleOk {
			cacheArticle := articleResult.(*Article)
			if (time.Now().Sub(cacheArticle.LastRetrieved)).Seconds() > MAX_TIME_ARTICLE.Seconds() {
				println("Cache expired for article: " + articleURL + "\nLast Retrieved: " + cacheArticle.LastRetrieved.String() + "\nDifference: " + time.Now().Sub(cacheArticle.LastRetrieved).String())
				article, err := sch.src.fetchArticle(articleURL)
				if err == nil {
					sch.articles.Store(articleURL, article)
					articles = append(articles, article)
				} else {
					// Article refresh failed — serve stale cached version
					// Update LastRetrieved to avoid retrying on every call
					stale := *cacheArticle
					stale.LastRetrieved = time.Now()
					sch.articles.Store(articleURL, &stale)
					articles = append(articles, &stale)
				}
			} else {
				println("Cache hit for article: " + articleURL)
				articles = append(articles, cacheArticle)
			}
		} else {
			// cache miss, query the article
			println("Cache miss for article: " + articleURL)
			article, err := sch.src.fetchArticle(articleURL)
			if err == nil {
				articles = append(articles, article)
				sch.articles.Store(articleURL, article)
			}
		}
	}
	return articles
}

func (sch *Scholar) QueryProfileWithMemoryCache(user string, limit int) ([]*Article, error) {

	profileResult, profileOk := sch.profile.Load(user)
	if profileOk {
		profile := profileResult.(Profile)
		lastAccess := profile.LastRetrieved
		if (time.Now().Sub(lastAccess)).Seconds() > MAX_TIME_PROFILE.Seconds() {
			println("Profile cache expired for User: " + user)
			// Only fetch the profile page (queryArticles=false) to get the
			// updated article list. Article details are served from cache
			// via loadCachedArticles, which refreshes only expired entries.
			profileArticles, err := sch.src.fetchProfile(user, limit, false)
			if err == nil {
				var articleList []string
				for _, article := range profileArticles {
					articleList = append(articleList, article.ScholarURL)
					// Update citation counts from the profile page into cached articles
					if existing, ok := sch.articles.Load(article.ScholarURL); ok {
						updated := *existing.(*Article)
						updated.NumCitations = article.NumCitations
						sch.articles.Store(article.ScholarURL, &updated)
					} else if sch.src.listingIsComplete() {
						// New to the cache and the listing already has full
						// details: seed it rather than fetching it separately.
						sch.articles.Store(article.ScholarURL, article)
					}
				}
				newProfile := Profile{User: user, LastRetrieved: time.Now(), Articles: articleList}
				sch.profile.Delete(user)
				sch.profile.Store(user, newProfile)
				return sch.loadCachedArticles(newProfile), nil
			} else {
				// Refresh failed (e.g. throttled) — fall back to stale cached data.
				// Update LastRetrieved to avoid retrying on every call.
				fmt.Printf("Profile refresh failed for %s: %v — serving stale cache\n", user, err)
				profile.LastRetrieved = time.Now()
				sch.profile.Store(user, profile)
				cached := sch.loadCachedArticles(profile)
				if len(cached) > 0 {
					return cached, nil
				}
				return nil, err
			}
		} else {
			println("Profile cache hit for User: " + user)
			return sch.loadCachedArticles(profile), nil
		}
	} else {
		println("Profile cache miss for User: " + user)
		// With nothing cached there is no stale data to fall back to, so a
		// failure here would otherwise turn every call into a new request.
		if f, ok := sch.inFailureCooldown(user); ok {
			return nil, fmt.Errorf("skipping fetch for %s, last attempt %s ago failed: %w", user, time.Since(f.at).Round(time.Second), f.err)
		}
		articles, err := sch.src.fetchProfile(user, limit, true)
		if err == nil {
			sch.clearFailure(user)
			var articleList []string
			for _, article := range articles {
				articleList = append(articleList, article.ScholarURL)
				// The listing carried full details, so seed the article cache
				// here rather than relying on the backend to have done it.
				sch.articles.Store(article.ScholarURL, article)
			}
			newProfile := Profile{User: user, LastRetrieved: time.Now(), Articles: articleList}
			sch.profile.Store(user, newProfile)
			return articles, nil
		} else {
			sch.recordFailure(user, err)
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

// isBlockPage reports whether a 403 body is Google's "automated queries"
// block page, as opposed to some other forbidden response.
func isBlockPage(body io.Reader) bool {
	b, err := io.ReadAll(io.LimitReader(body, 64*1024))
	if err != nil {
		return false
	}
	return bytes.Contains(b, []byte("automated queries"))
}

// inFailureCooldown returns the recorded failure for user if one happened
// less than failureCooldown ago. Expired entries are dropped when noticed so
// the map only ever holds users currently in cooldown.
func (sch *Scholar) inFailureCooldown(user string) (fetchFailure, bool) {
	sch.failureMu.Lock()
	defer sch.failureMu.Unlock()
	if sch.failureCooldown <= 0 {
		return fetchFailure{}, false
	}
	f, ok := sch.failures[user]
	if !ok {
		return fetchFailure{}, false
	}
	if time.Since(f.at) >= sch.failureCooldown {
		delete(sch.failures, user)
		return fetchFailure{}, false
	}
	return f, true
}

// recordFailure remembers a failed fetch for user; a no-op when the cooldown
// is disabled, so nothing accumulates that would never be read.
func (sch *Scholar) recordFailure(user string, err error) {
	sch.failureMu.Lock()
	defer sch.failureMu.Unlock()
	if sch.failureCooldown <= 0 {
		return
	}
	sch.failures[user] = fetchFailure{at: time.Now(), err: err}
}

func (sch *Scholar) clearFailure(user string) {
	sch.failureMu.Lock()
	defer sch.failureMu.Unlock()
	delete(sch.failures, user)
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
		if resp.StatusCode == 403 && isBlockPage(resp.Body) {
			return nil, fmt.Errorf("%s: %w", errorString, ErrBlocked)
		}
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
		article.ScholarURL = BaseURL + tempURL
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
