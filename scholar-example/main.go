package main

import (
	"flag"
	"fmt"
	scholar "github.com/compscidr/scholar"
)

func main() {
	userPtr := flag.String("user", "", "user profile to retrieve (Google Scholar id, or Semantic Scholar author id with -source semantic_scholar)")
	limitPtr := flag.Int("limit", 1, "limit the number of articles to retrieve")
	sourcePtr := flag.String("source", string(scholar.SourceGoogleScholar), "publication source: google_scholar or semantic_scholar")
	apiKeyPtr := flag.String("apikey", "", "Semantic Scholar API key (optional)")
	flag.Parse()

	if *userPtr == "" {
		flag.Usage()
		return
	}
	if *limitPtr < 1 {
		*limitPtr = 1
	}

	fmt.Println("Searching for user: " + *userPtr + " with limit: " + fmt.Sprint(*limitPtr))
	user := *userPtr
	limit := *limitPtr

	sch := scholar.New("profile.json", "articles.json")
	sch.SetSource(scholar.SourceKind(*sourcePtr))
	sch.SetAPIKey(*apiKeyPtr)
	//articles := sch.QueryProfileDumpResponse(user, limit, true)
	//articles := sch.QueryProfile(user, limit)
	articles, err := sch.QueryProfileWithMemoryCache(user, limit)

	if err != nil {
		fmt.Println(err)
		return
	}

	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range articles {
		fmt.Println(article)
	}

	cachedArticles, err := sch.QueryProfileWithMemoryCache(user, limit)
	if err != nil {
		fmt.Println(err)
	}
	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range cachedArticles {
		fmt.Println(article)
	}

	sch.SaveCache("profile.json", "articles.json")
	sch2 := scholar.New("profile.json", "articles.json")
	sch2.SetSource(scholar.SourceKind(*sourcePtr))
	sch2.SetAPIKey(*apiKeyPtr)
	cachedArticles2, err := sch2.QueryProfileWithMemoryCache(user, limit)
	if err != nil {
		fmt.Println(err)
		return
	}
	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range cachedArticles2 {
		fmt.Println(article)
	}
}
