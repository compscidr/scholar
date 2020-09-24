# scholar
scholar is a WiP Go module that implements a querier and parser for Google Scholar's output. Its classes can be used 
independently, but it can also be invoked as a command-line tool.

This tool is inspired by [scholar.py](https://github.com/ckreibich/scholar.py)

# Usage
```
import "github.com/compscidr/go-scholar/scholar"

articles := QueryProfile("SbUmSEAAAAAJ")

for _, article := range articles {
	// do something with the article
}
```

## Features
Working:
* Queries and parses a user profile by user id
* Extracts the publication title, url, year, number of citations, number of different versions of the same article,
and the URL to the citations. Still working on getting the excerpt, the URL to versions, and the clusterID.

TODO:
* Extracts publication title, most relevant web link, PDF link, number of citations, number of online versions, link to 
Google Scholar's article cluster for the work, Google Scholar's cluster of all works referencing the publication, and 
excerpt of content.
* Extracts total number of hits as reported by Scholar (new in version 2.5)
*  Supports the full range of advanced query options provided by Google Scholar, such as title-only search, publication 
date timeframes, and inclusion/exclusion of patents and citations.
*  Supports article cluster IDs, i.e., information relating to the variants of an article already identified by Google 
Scholar
*  Supports retrieval of citation details in standard external formats as provided by Google Scholar, including BibTeX 
and EndNote.
*  Command-line tool prints entries in CSV format, simple plain text, or in the citation export format.
*  Cookie support for higher query volume, including ability to persist cookies to disk across invocations.
