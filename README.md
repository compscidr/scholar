# scholar
scholar is a WiP Go module that implements a querier and parser for Google Scholar's output. Its classes can be used 
independently, but it can also be invoked as a command-line tool.

This tool is inspired by [scholar.py](https://github.com/ckreibich/scholar.py)

# Usage
```
import "github.com/compscidr/scholar"

sch := scholar.New("profiles.json", "articles.json")
articles := sch.QueryProfile("SbUmSEAAAAAJ", 1)

for _, article := range articles {
	// do something with the article
}
```

## Features
Working:
* Queries and parses a user profile by user id to get basic publication data
* Queries each of the articles listed (up to 80) and parses the results for extra information
* Caches the profile for a day, and articles for a week (need to confirm this is working)
  * This is in memory, so if the program is restarted, the cache is lost
* Configurable limit to number of articles to query in one go
* On-disk caching of the profile and articles to avoid hitting the rate limit

## TODO:
* Pagination of articles
* Add throttling to avoid hitting the rate limit (figure out what the limit is)

## Possible throttle info:
https://stackoverflow.com/questions/60271587/how-long-is-the-error-429-toomanyrequests-cooldown