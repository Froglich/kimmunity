package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type post struct {
	ID             string        `json:"post_id"`
	PostByUsername string        `json:"post_by_username"`
	PostByName     string        `json:"post_by_name"`
	Content        string        `json:"content"`
	Images         []string      `json:"images"`
	Comments       []postComment `json:"comments"`
	Likes          uint          `json:"likes"`
	LikedByMe      bool          `json:"liked_by_me"`
	Posted         uint64        `json:"posted"`
	MyPost         bool          `json:"my_post"`
}

type postComment struct {
	CommentID         uint   `json:"comment_id"`
	CommentByUsername string `json:"comment_by_username"`
	CommentByName     string `json:"comment_by_name"`
	Content           string `json:"content"`
	MyComment         bool   `json:"my_comment"`
	Timestamp         uint64 `json:"timestamp"`
}

type event struct {
	Type            string  `json:"type"`
	PostID          *string `json:"post_id,omitempty"`
	EventByUsername string  `json:"event_by_username"`
	EventByName     string  `json:"event_by_name"`
	Timestamp       uint64  `json:"timestamp"`
}

func bulkDeleteImageAssets(db *sql.DB, query string, params ...interface{}) error {
	rows, err := db.Query(query, params...)
	if err != nil {
		return err
	}

	var filename string
	for rows.Next() {
		if err := rows.Scan(&filename); err != nil {
			log.Println(err)
			continue
		}

		f := path.Join(settings.Storage, "images", filename)
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	return nil
}

func addNewPost(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		log.Panicln(err)
	}

	frm, err := reader.ReadForm(10000000)
	if err != nil {
		log.Panicln(err)
	}

	//Sanitize post text to remove any potential XSS and other garbage.
	//Then escape all HTML characters, since we dont want rich formatting.
	content := html.EscapeString(sanitizer.Sanitize(frm.Value["content"][0]))
	//Almost certainly overkill to sanitize and then escape.

	if len(content) < 1 {
		w.WriteHeader(http.StatusNotAcceptable)
		fmt.Fprintf(w, "empty content")
		return
	}

	postID := uuid.New().String()

	row := db.QueryRow("INSERT INTO posts(id, username, content) VALUES($1, $2, $3) RETURNING posted", postID, u.Username, content)
	var posted uint64
	if err := row.Scan(&posted); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	images, err := savePostImages(postID, db, frm)
	if err != nil {
		log.Println(err)
		//Continue anyway, the text content has been posted.
	}

	if len(images) > 0 {
		fmt.Fprintf(w, `{"post_id": "%s", "post_by_username": "%s", "post_by_name": "%s", "content": "%s", "posted": %d, "images": ["%s"], "likes": 0, "liked_by_me": false, "comments": [], "my_post": true}`, postID, u.Username, u.getName(), content, posted, strings.Join(images, `","`))
	} else {
		fmt.Fprintf(w, `{"post_id": "%s", "post_by_username": "%s", "post_by_name": "%s", "content": "%s", "posted": %d, "images": [], "likes": 0, "liked_by_me": false, "comments": [], "my_post": true}`, postID, u.Username, u.getName(), content, posted)
	}

}

func savePostImages(postID string, db *sql.DB, frm *multipart.Form) ([]string, error) {
	images := make([]string, 0)

	for f := range frm.File {
		_f := frm.File[f][0]
		file, err := _f.Open()
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer

		newFilename := fmt.Sprintf("%s.jpg", uuid.New())
		nfile := path.Join(settings.Storage, "images", newFilename)

		if _, err = os.Stat(nfile); err == nil {
			return nil, err
		} else if !os.IsNotExist(err) {
			return nil, err
		}

		outfile, err := os.Create(nfile)

		if err != nil {
			return nil, err
		}

		_, err = io.Copy(&buf, file)

		if err != nil {
			return nil, err
		}

		content := buf.Bytes()

		ct := _f.Header.Get("Content-Type")

		if ct != "image/jpeg" || content[0] != 255 || content[1] != 216 {
			return nil, fmt.Errorf("bad content-type: %s", ct)
		}

		_, err = outfile.Write(content)

		if err != nil {
			return nil, err
		}
		outfile.Sync()

		_, err = db.Exec("INSERT INTO images(post_id, filename) VALUES($1, $2)", postID, newFilename)

		if err != nil {
			return nil, err
		}

		file.Close()
		outfile.Close()

		images = append(images, newFilename)
	}

	return images, nil
}

func likePost(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	postID := mux.Vars(r)["post_id"]
	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("INSERT INTO post_likes(post_id, username) VALUES($1, $2)", postID, u.Username)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, "conflict")
	}
}

func unLikePost(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	postID := mux.Vars(r)["post_id"]
	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("DELETE FROM post_likes WHERE post_id = $1 AND username = $2", postID, u.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func commentOnPost(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	postID := mux.Vars(r)["post_id"]
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	comment := html.EscapeString(sanitizer.Sanitize(r.FormValue("comment")))

	row := db.QueryRow("INSERT INTO post_comments(post_id, username, content) VALUES($1, $2, $3) RETURNING id, tstamp", postID, u.Username, comment)
	var id uint
	var tstamp uint64
	if err := row.Scan(&id, &tstamp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	fmt.Fprintf(w, `{"comment_id": %d, "comment_by_username": "%s", "comment_by_name": "%s", "content": "%s", "my_comment": true, "timestamp": %d}`, id, u.Username, u.getName(), comment, tstamp)
}

func deleteComment(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	postID := mux.Vars(r)["post_id"]
	id := mux.Vars(r)["id"]

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	row := db.QueryRow("SELECT username FROM post_comments WHERE post_id = $1 AND id = $2", postID, id)
	var username string
	if err := row.Scan(&username); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if username != u.Username {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("DELETE FROM post_comments WHERE post_id = $1 AND id = $2", postID, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}
}

func getEvents(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	since := r.FormValue("since")
	var rows *sql.Rows
	var err error

	if since == "" {
		rows, err = db.Query("SELECT event_type, post_id, event_by_username, event_by_name, tstamp FROM view_events WHERE event_for = $1 ORDER BY tstamp LIMIT 20", u.Username)
	} else {
		rows, err = db.Query("SELECT event_type, post_id, event_by_username, event_by_name, tstamp FROM view_events WHERE event_for = $1 AND tstamp > $2::BIGINT ORDER BY tstamp LIMIT 20", u.Username, since)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	events := make([]event, 0)
	var eventType string
	var postID *string
	var eventByUsername string
	var eventByName string
	var timestamp uint64
	for rows.Next() {
		if err := rows.Scan(&eventType, &postID, &eventByUsername, &eventByName, &timestamp); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}

		events = append(events, event{
			Type:            eventType,
			PostID:          postID,
			EventByUsername: eventByUsername,
			EventByName:     eventByName,
			Timestamp:       timestamp,
		})
	}

	out, err := json.Marshal(events)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Write(out)
}

func (p *post) getPostImages(db *sql.DB) error {
	p.Images = make([]string, 0)
	rows, err := db.Query("SELECT filename FROM images i WHERE i.post_id = $1", p.ID)
	if err != nil {
		return err
	}

	var fname string
	for rows.Next() {
		if err := rows.Scan(&fname); err != nil {
			return err
		}
		p.Images = append(p.Images, fname)
	}

	return nil
}

func (p *post) getPostComments(db *sql.DB, u *user) error {
	p.Comments = make([]postComment, 0)

	rows, err := db.Query("SELECT id, comment_by_username, comment_by_name, content, tstamp FROM view_post_comments WHERE post_id = $1 ORDER BY id", p.ID)
	if err != nil {
		return err
	}

	var commentID uint
	var byUsername string
	var byName string
	var content string
	var timestamp uint64
	for rows.Next() {
		if err := rows.Scan(&commentID, &byUsername, &byName, &content, &timestamp); err != nil {
			return err
		}

		p.Comments = append(p.Comments, postComment{
			CommentID:         commentID,
			CommentByUsername: byUsername,
			CommentByName:     byName,
			Content:           content,
			Timestamp:         timestamp,
			MyComment:         byUsername == u.Username,
		})
	}

	return nil
}

func (p *post) getPostLikes(db *sql.DB, u *user) error {
	rows, err := db.Query("SELECT username FROM post_likes WHERE post_id = $1", p.ID)

	if err != nil {
		return err
	}

	var username string
	var likes uint = 0
	for rows.Next() {
		if err := rows.Scan(&username); err != nil {
			return err
		}

		likes++
		if username == u.Username {
			p.LikedByMe = true
		}
	}

	p.Likes = likes

	return nil
}

func getPost(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	postID := mux.Vars(r)["post_id"]

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	row := db.QueryRow("SELECT post_by_username, post_by_name, content, posted FROM view_posts WHERE post_id = $1", postID)
	var postByUsername string
	var postByName string
	var content string
	var posted uint64
	if err := row.Scan(&postByUsername, &postByName, &content, &posted); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	p := post{
		ID:             postID,
		PostByUsername: postByUsername,
		PostByName:     postByName,
		Content:        content,
		Posted:         posted,
		MyPost:         u.Username == postByUsername,
	}

	if err := p.getPostImages(db); err != nil {
		log.Println(err)
		//Continue anyway
	}

	if err := p.getPostComments(db, u); err != nil {
		log.Println(err)
		//Continue anyway
	}

	if err := p.getPostLikes(db, u); err != nil {
		log.Println(err)
		//Continue anyway
	}

	out, err := json.Marshal(p)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Write(out)
}

func deletePost(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	postID := mux.Vars(r)["post_id"]
	row := db.QueryRow("SELECT username FROM posts WHERE id = $1", postID)
	var username string
	if err := row.Scan(&username); err != nil {
		w.WriteHeader(http.StatusNotFound)
		log.Println(err)
		return
	}

	if username != u.Username {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	err := bulkDeleteImageAssets(db, "SELECT filename FROM images WHERE post_id = $1", postID)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = db.Exec("DELETE FROM posts WHERE id = $1", postID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}
}

func getPosts(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	rawBefore := r.FormValue("before")
	user := r.FormValue("user")
	var before uint64
	var err error
	before, err = strconv.ParseUint(rawBefore, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusNotAcceptable)
		log.Println(err)
		return
	}

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var sql string
	var params []interface{}

	if user == "" {
		sql = "SELECT post_by_username, post_by_name, content, post_id, posted FROM view_posts WHERE username = $1 AND posted < $2 ORDER BY posted DESC LIMIT 10"
		params = []interface{}{u.Username, before}
	} else {
		sql = sql + "SELECT u.username, COALESCE(u.name, u.username), p.content, p.id, p.posted FROM posts p LEFT JOIN users u ON u.username = p.username WHERE p.username = $1 AND posted < $2 ORDER BY posted DESC LIMIT 10"
		params = []interface{}{user, before}
	}

	rows, err := db.Query(sql, params...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	posts := make([]post, 0)
	var postByUsername string
	var postByName string
	var postID string
	var content string
	var posted uint64
	for rows.Next() {
		if err := rows.Scan(&postByUsername, &postByName, &content, &postID, &posted); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}

		p := post{
			PostByUsername: postByUsername,
			PostByName:     postByName,
			ID:             postID,
			Content:        content,
			Posted:         posted,
			MyPost:         postByUsername == u.Username,
		}

		if err := p.getPostImages(db); err != nil {
			log.Println(err)
			//Continue anyway
		}

		if err := p.getPostComments(db, u); err != nil {
			log.Println(err)
			//Continue anyway
		}

		if err := p.getPostLikes(db, u); err != nil {
			log.Println(err)
			//Continue anyway
		}

		posts = append(posts, p)
	}

	out, err := json.Marshal(posts)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	w.Write(out)
}
