package scholar

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"log"
	"net/http"
	"strconv"
)

const BaseURL = "http://scholar.google.com"
const AGENT = "Mozilla/5.0 (X11; Linux x86_64; rv:27.0) Gecko/20100101 Firefox/27.0"

type Article struct {
	title        string
	url          string
	year         int
	numCitations int
	numVersion   int
	clusterId    string
	urlCitations string
	urlVersions  string
	urlCitation  string
	excerpt      string
}

func QueryProfile(user string) []Article {
	var articles []Article
	// todo: make page size configurable
	client := &http.Client{}

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
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	doc.Find(".gsc_a_tr").Each(func(i int, s *goquery.Selection) {
		var article Article
		entry := s.Find(".gsc_a_t")
		link := entry.Find(".gsc_a_at")
		article.title = link.Text()
		tempURL, _ := link.Attr("data-href")
		article.year, _ = strconv.Atoi(s.Find(".gsc_a_y").Find("span").Text())
		article.numCitations, _ = strconv.Atoi(s.Find(".gsc_a_c").Find("a").Text())
		article.urlCitations, _ = s.Find(".gsc_a_c").Find("a").Attr("href")

		fmt.Println(BaseURL + tempURL)
		req2, err2 := http.NewRequest("GET", BaseURL+tempURL, nil)
		if err2 != nil {
			log.Fatalln(err)
		}
		req2.Header.Set("User-Agent", AGENT)
		resp2, err2 := client.Do(req2)
		if err2 != nil {
			log.Fatal(err2)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != 200 {
			log.Fatalf("status code error: %d %s", resp2.StatusCode, resp2.Status)
		}
		doc2, err2 := goquery.NewDocumentFromReader(resp2.Body)
		if err2 != nil {
			log.Fatal(err2)
		}
		article.url, _ = doc2.Find(".gsc_vcd_title_link").Attr("href")
		article.numVersion = 0
		doc2.Find("gs_scl").Each(func(i int, s *goquery.Selection) {
			text := s.Find("gsc_vcd_field").Text()
			if text == "Scholar articles" {
				s.Find("gsc_vcd_value").Each(func(i int, s *goquery.Selection) {
					article.numVersion = article.numVersion + 1
				})
			}
		})

		articles = append(articles, article)
	})

	return articles
}
