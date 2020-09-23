package scholar

import (
	"fmt"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestGetArticles(t *testing.T) {

}

func TestScholarQuerier(t *testing.T) {

}

func TestProfileQuerier(t *testing.T) {
	articles := QueryProfile("SbUmSEAAAAAJ")
	assert.NotEmpty(t, articles)

	for _, article := range articles {
		fmt.Println(article)
	}
}