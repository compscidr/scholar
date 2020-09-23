package scholar

import (
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
)

const BaseURL = "http://scholar.google.com"

type Article struct {
	title string
	url string
	year int
	numCitations int
	numVersion int
	clusterId string
	urlCitations string
	urlVersions string
	urlCitation string
	excerpt string
}

func QueryProfile(user string) [] Article {
	var articles []Article
	// todo: make page size configurable
	resp, err := http.Get(BaseURL + "/citations?user=" + user + "&cstart=0&pagesize=80")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("status code error: %d %s", resp.StatusCode, resp.Status)
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
		article.url, _ = link.Attr("data-href")
		articles = append(articles, article)
	})

	return articles
}
