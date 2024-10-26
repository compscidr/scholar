package main

import (
	"flag"
	"fmt"
	scholar "github.com/compscidr/scholar"
)

func main() {
	userPtr := flag.String("user", "", "user profile to retrieve")
	limitPtr := flag.Int("limit", 1, "limit the number of articles to retrieve")
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
	//articles := sch.QueryProfileDumpResponse(user, limit, true)
	//articles := sch.QueryProfile(user, limit)
	articles := sch.QueryProfileWithMemoryCache(user, limit)

	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range articles {
		fmt.Println(article)
	}

	cachedArticles := sch.QueryProfileWithMemoryCache(user, limit)
	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range cachedArticles {
		fmt.Println(article)
	}

	sch.SaveCache("profile.json", "articles.json")
	sch2 := scholar.New("profile.json", "articles.json")
	cachedArticles2 := sch2.QueryProfileWithMemoryCache(user, limit)
	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range cachedArticles2 {
		fmt.Println(article)
	}
}
