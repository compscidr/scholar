package go_scholar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const BaseURL = "http://scholar.google.com"
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
	articles sync.Map // map of articles by URL
	profile  sync.Map // map of profile by User string
}

func New(profileCache string, articleCache string) *Scholar {

	profileFile, err := os.Open(profileCache)
	if err != nil {
		println("Error opening profile cache file: " + profileCache + " - creating new cache")
		return &Scholar{}
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
		return &Scholar{}
	}

	articleFile, err := os.Open(articleCache)
	if err != nil {
		println("Error opening article cache file: " + articleCache + " - creating new cache")
		return &Scholar{}
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			println("Error closing article cache file: " + articleCache)
		}
	}(articleFile)
	articleDecoder := json.NewDecoder(articleFile)
	var regularArticleMap map[string]Article
	err = articleDecoder.Decode(&regularArticleMap)
	if err != nil {
		println("Error decoding article cache file: " + articleCache + " - creating new cache")
		return &Scholar{}
	}

	sch := Scholar{}

	// convert the regular maps to sync maps
	for key, value := range regularProfileMap {
		sch.profile.Store(key, value)
	}
	for key, value := range regularArticleMap {
		sch.articles.Store(key, value)
	}

	return &sch
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
}

func (a Article) String() string {
	return "Article(\n  Title=" + a.Title + "\n  authors=" + a.Authors + "\n  ScholarURL=" + a.ScholarURL + "\n  Year=" + strconv.Itoa(a.Year) + "\n  Month=" + strconv.Itoa(a.Month) + "\n  Day=" + strconv.Itoa(a.Day) + "\n  NumCitations=" + strconv.Itoa(a.NumCitations) + "\n  Articles=" + strconv.Itoa(a.Articles) + "\n  Description=" + a.Description + "\n  PdfURL=" + a.PdfURL + "\n  Journal=" + a.Journal + "\n  Volume=" + a.Volume + "\n  Pages=" + a.Pages + "\n  Publisher=" + a.Publisher + "\n  scholarCitedByURL=" + strings.Join(a.ScholarCitedByURLs, ", ") + "\n  scholarVersionsURL=" + strings.Join(a.ScholarVersionsURLs, ", ") + "\n  scholarRelatedURL=" + strings.Join(a.ScholarRelatedURLs, ", ") + "\n  LastRetrieved=" + a.LastRetrieved.String() + "\n)"
}

func (sch *Scholar) QueryProfile(user string, limit int) []Article {
	return sch.QueryProfileDumpResponse(user, true, limit, false)
}

func (sch *Scholar) QueryProfileWithMemoryCache(user string, limit int) []Article {

	profileResult, profileOk := sch.profile.Load(user)
	if profileOk {
		profile := profileResult.(Profile)
		lastAccess := profile.LastRetrieved
		if (time.Now().Sub(lastAccess)).Seconds() > MAX_TIME_PROFILE.Seconds() {
			println("Profile cache expired for User: " + user)
			sch.profile.Delete(user)
			articles := sch.QueryProfileDumpResponse(user, true, limit, false)
			var articleList []string
			for _, article := range articles {
				articleList = append(articleList, article.ScholarURL)
			}
			newProfile := Profile{User: user, LastRetrieved: time.Now(), Articles: articleList}
			sch.profile.Store(user, newProfile)
		} else {
			println("Profile cache hit for User: " + user)
			// cache hit, return the Articles
			articles := make([]Article, 0)
			for _, articleURL := range profile.Articles {
				articleResult, articleOk := sch.articles.Load(articleURL)
				if articleOk {
					cacheArticle := articleResult.(Article)
					if (time.Now().Sub(cacheArticle.LastRetrieved)).Seconds() > MAX_TIME_ARTICLE.Seconds() {
						println("Cache expired for article: " + articleURL + "\nLast Retrieved: " + cacheArticle.LastRetrieved.String() + "\nDifference: " + time.Now().Sub(cacheArticle.LastRetrieved).String())
						article := sch.QueryArticle(articleURL, Article{}, false)
						sch.articles.Store(articleURL, article)
						articles = append(articles, article)
					} else {
						println("Cache hit for article: " + articleURL)
						articles = append(articles, cacheArticle)
					}
				} else {
					// cache miss, query the article
					println("Cache miss for article: " + articleURL)
					article := sch.QueryArticle(articleURL, Article{}, false)
					articles = append(articles, article)
					sch.articles.Store(articleURL, article)
				}
			}
			return articles
		}

	} else {
		println("Profile cache miss for User: " + user)
		articles := sch.QueryProfileDumpResponse(user, true, limit, false)
		var articleList []string
		for _, article := range articles {
			articleList = append(articleList, article.ScholarURL)
		}
		newProfile := Profile{User: user, LastRetrieved: time.Now(), Articles: articleList}
		sch.profile.Store(user, newProfile)
		return articles
	}

	println("Shouldn't have got here")
	return []Article{}
}

// QueryProfileDumpResponse queries the profile of a User and returns a list of Articles
// if queryArticles is true, it will also query the Articles for extra information which isn't present on the profile page
//
//	we may wish to set this to false if we are only interested in some article info, or we have a cache hit and we just
//	want to get updated information from the profile page only to save requests
//
// if dumpResponse is true, it will print the response to stdout (useful for debugging)
func (sch *Scholar) QueryProfileDumpResponse(user string, queryArticles bool, limit int, dumpResponse bool) []Article {
	var articles []Article
	client := &http.Client{}

	// todo: make page size configurable, also support getting more than one page of citations
	req, err := http.NewRequest("GET", BaseURL+"/citations?user="+user+"&cstart=0&pagesize="+strconv.Itoa(limit), nil)
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Set("User-Agent", AGENT)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	if dumpResponse {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		println("GOT AUTHOR PAGE: \n", string(bodyBytes))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	doc.Find(".gsc_a_tr").Each(func(i int, s *goquery.Selection) {
		var article Article
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
				cacheArticle := articleResult.(Article)
				if (time.Now().Sub(article.LastRetrieved)).Seconds() > MAX_TIME_ARTICLE.Seconds() {
					println("Cache expired for article" + BaseURL + tempURL + "\nLast Retrieved: " + cacheArticle.LastRetrieved.String() + "\nDifference: " + time.Now().Sub(cacheArticle.LastRetrieved).String())
					// expired cache entry, replace it
					sch.articles.Delete(BaseURL + tempURL)
					article = sch.QueryArticle(BaseURL+tempURL, article, dumpResponse)
					sch.articles.Store(BaseURL+tempURL, article)
				} else {
					println("Cache hit for article" + BaseURL + tempURL)
					// not expired, update any new information
					cacheArticle.NumCitations = article.NumCitations // update the citations since thats all that might change
					article = cacheArticle
					sch.articles.Store(BaseURL+tempURL, article)
				}
			} else {
				println("Cache miss for article" + BaseURL + tempURL)
				article = sch.QueryArticle(BaseURL+tempURL, article, dumpResponse)
				sch.articles.Store(BaseURL+tempURL, article)
			}
		}
		articles = append(articles, article)
	})

	return articles
}

func (sch *Scholar) QueryArticle(url string, article Article, dumpResponse bool) Article {
	fmt.Println("PULLING ARTICLE: " + url)
	article.ScholarURL = url
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalln(err)
	}
	req.Header.Set("User-Agent", AGENT)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	if dumpResponse {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		println("GOT ARTICLE: \n", string(bodyBytes))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
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
	return article
}
