package main

import (
	"./scholar"
	"flag"
	"fmt"
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
	articles := scholar.QueryProfile(user)

	if len(articles) == 0 {
		fmt.Println("Not found")
		return
	}

	for _, article := range articles {
		fmt.Println(article)
	}
}
