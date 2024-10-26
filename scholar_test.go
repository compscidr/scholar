package go_scholar

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetArticles(t *testing.T) {

}

func TestScholarQuerier(t *testing.T) {

}

func TestProfileQuerier(t *testing.T) {
	sch := New("profiles.json", "articles.json")
	articles := sch.QueryProfile("SbUmSEAAAAAJ", 1)
	assert.NotEmpty(t, articles)

	for _, article := range articles {
		fmt.Println(article)
	}
}
