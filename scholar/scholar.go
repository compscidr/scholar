package scholar

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const BaseURL = "http://scholar.google.com"
const AGENT = "Mozilla/5.0 (X11; Linux x86_64; rv:60.0) Gecko/20100101 Firefox/81.0"

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

func (a Article) String() string {
	return "Article(\n  title=" + a.title + "\n  authors=" + a.authors + "\n  scholarURL=" + a.scholarURL + "\n  year=" + strconv.Itoa(a.year) + "\n  month=" + strconv.Itoa(a.month) + "\n  day=" + strconv.Itoa(a.day) + "\n  numCitations=" + strconv.Itoa(a.numCitations) + "\n  articles=" + strconv.Itoa(a.articles) + "\n  description=" + a.description + "\n  pdfURL=" + a.pdfURL + "\n  journal=" + a.journal + "\n  volume=" + a.volume + "\n  pages=" + a.pages + "\n  publisher=" + a.publisher + "\n  scholarCitedByURL=" + strings.Join(a.scholarCitedByURLs, ", ") + "\n  scholarVersionsURL=" + strings.Join(a.scholarVersionsURLs, ", ") + "\n  scholarRelatedURL=" + strings.Join(a.scholarRelatedURLs, ", ") + "\n  lastRetrieved=" + a.lastRetrieved.String() + "\n)"
}

func QueryProfile(user string) []Article {
	return QueryProfileDumpResponse(user, false)
}

func QueryProfileDumpResponse(user string, dumpResponse bool) []Article {
	var articles []Article
	client := &http.Client{}

	// todo: make page size configurable, also support getting more than one page of citations
	req, err := http.NewRequest("GET", BaseURL+"/citations?user="+user+"&cstart=0&pagesize=80", nil)
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

		// go to the article detail page to get rest of info (perhaps we want to make this configurable if someone
		// doesn't need the extra data and wants to save on requests
		article = QueryArticle(BaseURL+tempURL, article, dumpResponse)
		articles = append(articles, article)
	})

	return articles
}

func QueryArticle(url string, article Article, dumpResponse bool) Article {
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
				// each one of these is an article. For an example with multiple see: https://scholar.google.com/citations?view_op=view_citation&hl=en&user=ECQMeb0AAAAJ&citation_for_view=ECQMeb0AAAAJ:u5HHmVD_uO8C
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
