package main

import (
	"flag"
	"fmt"
	"github.com/compscidr/scholar"
)

func main() {
	userPtr := flag.String("user", "", "user profile to retrieve")
	flag.Parse()

	if *userPtr == "" {
		flag.Usage()
		return
	}

	fmt.Println("Searching for user: " + *userPtr)
	user := *userPtr

	sch := scholar.New()
	//articles := sch.QueryProfileDumpResponse(user, true)
	articles := sch.QueryProfile(user)

	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range articles {
		fmt.Println(article)
	}
}
