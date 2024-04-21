package go_scholar

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	cmap "github.com/orcaman/concurrent-map/v2"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "http://scholar.google.com"
const AGENT = "Mozilla/5.0 (X11; Linux x86_64; rv:60.0) Gecko/20100101 Firefox/81.0"
const MAX_TIME_PROFILE = time.Second * 3600 * 24     // 1 day
const MAX_TIME_ARTICLE = time.Second * 3600 * 24 * 7 // 1 week

type Article struct {
	title               string
	authors             string
	scholarURL          string
	year                int
	month               int
	day                 int
	numCitations        int
	articles            int // if there are more than one article within this publication (it will also tell how big the arrays below are)
	description         string
	pdfURL              string
	journal             string
	volume              string
	pages               string
	publisher           string
	scholarCitedByURLs  []string
	scholarVersionsURLs []string
	scholarRelatedURLs  []string
	lastRetrieved       time.Time
}

type Profile struct {
	user          string
	lastRetrieved time.Time
	articles      []string // list of article URLs - we'd still need to look them up in the article map
}

type Scholar struct {
	articles cmap.ConcurrentMap[string, Article] // map of articles by URL
	profile  cmap.ConcurrentMap[string, Profile] // map of profile by user string
}

func New() Scholar {
	return Scholar{
		articles: cmap.New[Article](),
		profile:  cmap.New[Profile](),
	}
}

func (a Article) String() string {
	return "Article(\n  title=" + a.title + "\n  authors=" + a.authors + "\n  scholarURL=" + a.scholarURL + "\n  year=" + strconv.Itoa(a.year) + "\n  month=" + strconv.Itoa(a.month) + "\n  day=" + strconv.Itoa(a.day) + "\n  numCitations=" + strconv.Itoa(a.numCitations) + "\n  articles=" + strconv.Itoa(a.articles) + "\n  description=" + a.description + "\n  pdfURL=" + a.pdfURL + "\n  journal=" + a.journal + "\n  volume=" + a.volume + "\n  pages=" + a.pages + "\n  publisher=" + a.publisher + "\n  scholarCitedByURL=" + strings.Join(a.scholarCitedByURLs, ", ") + "\n  scholarVersionsURL=" + strings.Join(a.scholarVersionsURLs, ", ") + "\n  scholarRelatedURL=" + strings.Join(a.scholarRelatedURLs, ", ") + "\n  lastRetrieved=" + a.lastRetrieved.String() + "\n)"
}

func (sch Scholar) QueryProfile(user string, limit int) []Article {
	return sch.QueryProfileDumpResponse(user, true, limit, false)
}

func (sch Scholar) QueryProfileWithCache(user string, limit int) []Article {
	if sch.profile.Has(user) {
		p, _ := sch.profile.Get(user)
		lastAccess := p.lastRetrieved
		if (time.Now().Sub(lastAccess)).Seconds() > MAX_TIME_PROFILE.Seconds() {
			println("Profile cache expired for user: " + user)
			sch.profile.Remove(user)
			articles := sch.QueryProfileDumpResponse(user, true, limit, false)
			var articleList []string
			for _, article := range articles {
				articleList = append(articleList, article.scholarURL)
			}
			sch.profile.Set(user, Profile{user: user, lastRetrieved: time.Now(), articles: articleList})
		} else {
			println("Profile cache hit for user: " + user)
			// cache hit, return the articles
			articles := make([]Article, 0)
			for _, articleURL := range p.articles {
				if sch.articles.Has(articleURL) {
					cacheArticle, _ := sch.articles.Get(articleURL)
					if (time.Now().Sub(cacheArticle.lastRetrieved)).Seconds() > MAX_TIME_ARTICLE.Seconds() {
						println("Cache expired for article: " + articleURL + "\nLast Retrieved: " + cacheArticle.lastRetrieved.String() + "\nDifference: " + time.Now().Sub(cacheArticle.lastRetrieved).String())
						article := sch.QueryArticle(articleURL, Article{}, false)
						sch.articles.Set(articleURL, article)
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
					sch.articles.Set(articleURL, article)
				}
			}
			return articles
		}

	} else {
		println("Profile cache miss for user: " + user)
		articles := sch.QueryProfileDumpResponse(user, true, limit, false)
		var articleList []string
		for _, article := range articles {
			articleList = append(articleList, article.scholarURL)
		}
		sch.profile.Set(user, Profile{user: user, lastRetrieved: time.Now(), articles: articleList})
		return articles
	}

	println("Shouldn't have got here")
	return []Article{}
}

// QueryProfileDumpResponse queries the profile of a user and returns a list of articles
// if queryArticles is true, it will also query the articles for extra information which isn't present on the profile page
//
//	we may wish to set this to false if we are only interested in some article info, or we have a cache hit and we just
//	want to get updated information from the profile page only to save requests
//
// if dumpResponse is true, it will print the response to stdout (useful for debugging)
func (sch Scholar) QueryProfileDumpResponse(user string, queryArticles bool, limit int, dumpResponse bool) []Article {
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
		article.title = link.Text()

		tempURL, _ := link.Attr("href")
		article.year, _ = strconv.Atoi(s.Find(".gsc_a_y").Find("span").Text())
		article.numCitations, _ = strconv.Atoi(s.Find(".gsc_a_c").Children().First().Text())

		if queryArticles {
			if sch.articles.Has(BaseURL + tempURL) {
				// hit the cache
				cacheArticle, _ := sch.articles.Get(BaseURL + tempURL)
				if (time.Now().Sub(article.lastRetrieved)).Seconds() > MAX_TIME_ARTICLE.Seconds() {
					println("Cache expired for article" + BaseURL + tempURL + "\nLast Retrieved: " + cacheArticle.lastRetrieved.String() + "\nDifference: " + time.Now().Sub(cacheArticle.lastRetrieved).String())
					// expired cache entry, replace it
					sch.articles.Remove(BaseURL + tempURL)
					article = sch.QueryArticle(BaseURL+tempURL, article, dumpResponse)
					sch.articles.Set(BaseURL+tempURL, article)
				} else {
					println("Cache hit for article" + BaseURL + tempURL)
					// not expired, update any new information
					cacheArticle.numCitations = article.numCitations // update the citations since thats all that might change
					article = cacheArticle
					sch.articles.Set(BaseURL+tempURL, article)
				}
			} else {
				println("Cache miss for article" + BaseURL + tempURL)
				article = sch.QueryArticle(BaseURL+tempURL, article, dumpResponse)
				sch.articles.Set(BaseURL+tempURL, article)
			}
		}
		articles = append(articles, article)
	})

	return articles
}

func (sch Scholar) QueryArticle(url string, article Article, dumpResponse bool) Article {
	fmt.Println("PULLING ARTICLE: " + url)
	article.scholarURL = url
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
	article.lastRetrieved = time.Now()
	article.articles = 0
	article.pdfURL, _ = doc.Find(".gsc_oci_title_ggi").Children().First().Attr("href") // assume the link is the first child
	doc.Find(".gs_scl").Each(func(i int, s *goquery.Selection) {
		text := s.Find(".gsc_oci_field").Text()
		if text == "Authors" {
			article.authors = s.Find(".gsc_oci_value").Text()
		}
		if text == "Publication date" {
			datestring := s.Find(".gsc_oci_value").Text()
			parts := strings.Split(datestring, "/")
			if len(parts) == 3 {
				article.year, _ = strconv.Atoi(parts[0])
				article.month, _ = strconv.Atoi(parts[1])
				article.day, _ = strconv.Atoi(parts[2])
			}
		}
		if text == "Journal" {
			article.journal = s.Find(".gsc_oci_value").Text()
		}
		if text == "Volume" {
			article.volume = s.Find(".gsc_oci_value").Text()
		}
		if text == "Pages" {
			article.pages = s.Find(".gsc_oci_value").Text()
		}
		if text == "Publisher" {
			article.publisher = s.Find(".gsc_oci_value").Text()
		}
		if text == "Description" {
			article.description = s.Find(".gsc_oci_value").Text()
		}
		// don't need to parse here, already have it
		//if text == "Total citations" {
		//	citationString := s.Find(".gsc_oci_value").Text()
		//	parts := strings.Split(citationString, "Cited by ")
		//	if len(parts) == 2 {
		//		article.numCitations, _ = strconv.Atoi(parts[1])
		//	}
		//}
		if text == "Scholar articles" {
			article.articles += 1
			articles := s.Find(".gsc_oci_value")
			articles.Find(".gsc_oci_merged_snippet").Each(func(i int, s *goquery.Selection) {
				// each one of these is an article. For an scholar-example with multiple see: https://scholar.google.com/citations?view_op=view_citation&hl=en&user=ECQMeb0AAAAJ&citation_for_view=ECQMeb0AAAAJ:u5HHmVD_uO8C
				// this seems to happen if the entry is a book and there are articles within it
				s.Find(".gsc_oms_link").Each(func(i int, l *goquery.Selection) {
					linkText := l.Text()
					linkUrl, _ := l.Attr("href")
					if strings.Contains(linkText, "Cited by") {
						article.scholarCitedByURLs = append(article.scholarCitedByURLs, linkUrl)
					}
					if strings.Contains(linkText, "Related articles") {
						article.scholarRelatedURLs = append(article.scholarRelatedURLs, linkUrl)
					}
					if strings.Contains(linkText, "versions") {
						article.scholarVersionsURLs = append(article.scholarVersionsURLs, linkUrl)
					}
				})
			})
		}
	})
	return article
}
