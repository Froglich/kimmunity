package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

func unauthenticatedView(view string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open(path.Join(settings.ConfDir, "views", view))

		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			log.Panicln(err)
		}

		_, err = io.Copy(w, f)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Panicln(err)
		}
	}
}

func authenticatedView(view string) func(http.ResponseWriter, *http.Request) {
	db := dbConnection()
	defer db.Close()

	return func(w http.ResponseWriter, r *http.Request) {
		db := dbConnection()
		defer db.Close()

		if u := requireCurrentUser(db, w, r); u == nil {
			return
		}

		f, err := os.Open(path.Join(settings.ConfDir, "views", view))

		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			log.Panicln(err)
		}

		_, err = io.Copy(w, f)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Panicln(err)
		}
	}
}

type pageSummary struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Image       *string `json:"image"`
}

func (ps *pageSummary) toJSON() []byte {
	out, _ := json.Marshal(ps)
	return out
}

// The following functions try to extract information from websites using regex. It works pretty well, but
// a more solid alternative would definitely be to parse the HTML and read the attributes.
func getPageTitle(data []byte) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`<meta.* property=\"og:title\"[^><]*content=\"([^><\"]*)\".*\/?>`),
		regexp.MustCompile(`<meta.* property=\"twitter:title\"[^><]*content=\"([^><\"]*)\".*\/?>`),
		regexp.MustCompile(`<title.*\>\s*?(.+)\s*?<\/title>`),
	}

	for x := range patterns {
		p := patterns[x]
		title := p.FindSubmatch(data)
		if len(title) >= 2 {
			return sanitizer.Sanitize(string(title[1]))
		}
	}

	return ""
}

func getPageDescription(data []byte) *string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`<meta.* property=\"og:description\"[^><]*content=\"([^><\"]*)\".*\/?>`),
		regexp.MustCompile(`<meta.* property=\"twitter:description\"[^><]*content=\"([^><\"]*)\".*\/?>`),
		regexp.MustCompile(`<p.*\>\s*?(.+)\s*?<\/p>`),
	}

	for x := range patterns {
		p := patterns[x]
		title := p.FindSubmatch(data)
		if len(title) >= 2 {
			v := sanitizer.Sanitize(string(title[1]))
			return &v
		}
	}

	return nil
}

func getPageImage(data []byte) *string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`<meta.* property=\"og:image\"[^><]*content=\"((?:https?:\/\/)?(?:[a-zA-Z0-9-]+\.)+[a-z]+(?::[0-9]+)?(?:\/[^\s,]+)*)\".*\/?>`),
		regexp.MustCompile(`<meta.* property=\"twitter:image\"[^><]*content=\"((?:https?:\/\/)?(?:[a-zA-Z0-9-]+\.)+[a-z]+(?::[0-9]+)?(?:\/[^\s,]+)*)\".*\/?>`),
		regexp.MustCompile(`<img[^><]*src=\"((?:https?:\/\/)?(?:[a-zA-Z0-9-]+\.)+[a-z]+(?::[0-9]+)?(?:\/[^\s,]+)*)\".*\/?>`),
	}

	//Image links from Aftonbladet had some strange syntax
	pClean := regexp.MustCompile(`&(?:amp;)+`)

	for x := range patterns {
		p := patterns[x]
		title := p.FindSubmatch(data)
		if len(title) >= 2 {
			//Not sanitizing here to avoid invalidating the URL, lets hope the matching regex was specific enough
			v := pClean.ReplaceAllString(string(title[1]), "&")
			return &v
		}
	}

	return nil
}

func getPageSummaryByRequest(db *sql.DB, url string) (*pageSummary, error) {
	if !strings.HasPrefix(url, "http") {
		url = fmt.Sprintf("https://%s", url)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	c := &http.Client{}
	response, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("error %d", response.StatusCode)
	}

	//Would very likely be wise to limit this to the head/(first 1kb or so) only.
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	ps := pageSummary{
		Title:       getPageTitle(data),
		Description: getPageDescription(data),
		Image:       getPageImage(data),
	}

	_, err = db.Exec("INSERT INTO page_summaries(url, title, description, image) VALUES($1, $2, $3, $4)", url, ps.Title, ps.Description, ps.Image)
	if err != nil {
		return nil, err
	}

	return &ps, nil
}

func getPageSummaryFromDB(db *sql.DB, url string) (*pageSummary, error) {
	row := db.QueryRow("SELECT title, description, image FROM page_summaries WHERE url = $1", url)

	var title string
	var description *string
	var image *string
	if err := row.Scan(&title, &description, &image); err != nil {
		return nil, err
	}

	ps := pageSummary{
		Title:       title,
		Description: description,
		Image:       image,
	}

	return &ps, nil
}

func getPageSummary(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	url := r.FormValue("url")
	if url == "" {
		w.WriteHeader(http.StatusFailedDependency)
		return
	}

	var ps *pageSummary
	var err error

	ps, err = getPageSummaryFromDB(db, url)
	if err == nil {
		w.Write(ps.toJSON())
		return
	}

	ps, err = getPageSummaryByRequest(db, url)
	if err == nil {
		w.Write(ps.toJSON())
		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	log.Println(err)
}
