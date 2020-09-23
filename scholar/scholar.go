package scholar

import (
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

func QueryProfile(user string) {
	// todo: make page size configurable
	resp, err := http.Get(BaseURL + "/citations?user=" + user + "&cstart=0&pagesize=80")
	if err != nil {
		log.Print(err)
	}
	log.Print(resp)
}

func Test() {
	println("TEST")
}
