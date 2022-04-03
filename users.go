package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
	"golang.org/x/crypto/bcrypt"
)

type user struct {
	Username   string  `json:"username"`
	Name       *string `json:"name"`
	ProfilePic bool    `json:"profile_picture"`
	Email      string  `json:"-"`
}

type userInfo struct {
	BaseInfo     user `json:"base_info"`
	Followers    uint `json:"followers"`
	Following    uint `json:"following"`
	FollowedByMe bool `json:"followed_by_me"`
	IsMe         bool `json:"is_me"`
}

func myProfile(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/users/%s", u.Username), http.StatusSeeOther)
}

func myProfilePicture(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/backend/users/%s/profile-picture", u.Username), http.StatusSeeOther)
}

func (u *user) getName() string {
	if u.Name != nil {
		return *u.Name
	}

	return u.Username
}

func (u *user) setName(db *sql.DB, name string) error {
	var err error
	if name == "" {
		_, err = db.Exec("UPDATE users SET name = NULL WHERE username = $1", u.Username)
	} else {
		_, err = db.Exec("UPDATE users SET name = $1 WHERE username = $2", name, u.Username)
	}

	return err
}

func getSessionIdentifier(r *http.Request) string {
	cookies := r.Cookies()

	for c := range cookies {
		if cookies[c].Name == "KONVERGENS" {
			return cookies[c].Value
		}
	}

	return ""
}

func checkCredentials(db *sql.DB, login string, password string) *user {
	row := db.QueryRow("SELECT username, email, pwhash FROM users WHERE username = $1 OR email = $1", login)

	var username string
	var email string
	var hash string
	var u user

	if err := row.Scan(&username, &email, &hash); err == nil {
		comparison := []byte(password)
		dbhash := []byte(hash)

		if err := bcrypt.CompareHashAndPassword(dbhash, comparison); err == nil {
			u.Username = username
			u.Email = email
			return &u
		}
	}

	return nil
}

func getCurrentUser(db *sql.DB, r *http.Request) *user {
	identifier := getSessionIdentifier(r)

	if identifier != "" {
		row := db.QueryRow("SELECT u.username, u.name, u.email, u.profile_picture FROM valid_sessions s LEFT JOIN users u ON u.username = s.username WHERE session_id = $1", identifier)

		var username string
		var email string
		var name *string
		var pic bool
		err := row.Scan(&username, &name, &email, &pic)
		if err != nil {
			return nil
		}

		return &user{
			Username:   username,
			Name:       name,
			Email:      email,
			ProfilePic: pic,
		}
	}

	return nil
}

func setDisplayName(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	nn := r.FormValue("name")
	pName := regexp.MustCompile(`^(?:[A-Za-z0-9ÅåÄäÖö]+ ?)*[A-Za-z0-9ÅåÄäÖö]$`)
	if !pName.MatchString(nn) {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	err := u.setName(db, nn)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
	}
}

func delDisplayName(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	err := u.setName(db, "")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
	}
}

func getUserProfilePicture(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	username := mux.Vars(r)["username"]

	row := db.QueryRow("SELECT profile_picture FROM users WHERE username = $1", username)
	var pp bool
	if err := row.Scan(&pp); err != nil {
		w.WriteHeader(http.StatusNotFound)
	}

	if !pp {
		f, err := os.Open(path.Join(settings.ConfDir, "static", "images", "default_pic.png"))
		if err != nil {
			log.Panicln(err)
		}

		io.Copy(w, f)
	} else {
		f, err := os.Open(path.Join(settings.Storage, "images", fmt.Sprintf("%s.jpg", username)))
		if err != nil {
			log.Panicln(err)
		}

		io.Copy(w, f)
	}
}

func setUserProfilePicture(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	username := mux.Vars(r)["username"]
	if u.Username != username {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		panic(err)
	}

	frm, err := reader.ReadForm(10000000)
	if err != nil {
		panic(err)
	}

	var fh []*multipart.FileHeader
	var ok bool
	fh, ok = frm.File["profile-picture"]
	if !ok {
		w.WriteHeader(http.StatusFailedDependency)
		return
	}

	f := fh[0]
	file, err := f.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	ct := f.Header.Get("Content-Type")
	if ct != "image/jpeg" {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	img, err := jpeg.Decode(file)
	if err != nil {
		w.WriteHeader(http.StatusNotAcceptable)
		log.Println(err)
		return
	}

	rz := resize.Thumbnail(400, 400, img, resize.Bicubic)

	nfile := path.Join(settings.Storage, "images", fmt.Sprintf("%s.jpg", username))
	outfile, err := os.Create(nfile)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	err = jpeg.Encode(outfile, rz, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	_, err = db.Exec("UPDATE users SET profile_picture = TRUE WHERE username = $1", username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
	}
}

func delUserProfilePicture(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	username := mux.Vars(r)["username"]
	if u.Username != username {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	fpath := path.Join(settings.Storage, "images", fmt.Sprintf("%s.jpg", username))

	os.Remove(fpath)

	_, err := db.Exec("UPDATE users SET profile_picture = FALSE WHERE username = $1", username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
	}
}

func requireCurrentUser(db *sql.DB, w http.ResponseWriter, r *http.Request) *user {
	if u := getCurrentUser(db, r); u != nil {
		return u
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)

	return nil
}

func createAccount(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()
	username := strings.ToLower(r.FormValue("username"))
	userEmail := strings.ToLower(r.FormValue("email"))

	rawName := r.FormValue("name")
	var name *string
	if rawName != "" {
		name = &rawName
	}

	rn, _ := rand.Prime(rand.Reader, 64)
	hashsum := sha256.Sum256([]byte(fmt.Sprintf("%d%d", time.Now().UTC().UnixNano(), rn)))
	hexstring := hex.EncodeToString(hashsum[:])

	mailPattern := regexp.MustCompile(`^[^@]+@[\w\.\-\"]+\.[a-z\.]+[^\.]$`)
	usernamePatter := regexp.MustCompile(`^[a-z0-9\_\-]{3,}$`)

	if !mailPattern.MatchString(userEmail) {
		w.WriteHeader(http.StatusNotAcceptable)
		fmt.Fprint(w, "e-mail")
		return
	}

	if !usernamePatter.MatchString(username) {
		w.WriteHeader(http.StatusNotAcceptable)
		fmt.Fprint(w, "bad username")
		return
	}

	_, err := db.Exec("INSERT INTO users(username, email, name) VALUES ($1, $2, $3)", username, userEmail, name)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, "e-mail or name")
		return
	}

	_, err = db.Exec("INSERT INTO account_keys(username, key) VALUES($1, $2)", username, hexstring)

	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "key")
		return
	}

	mail := email{SenderAddress: settings.Email.Username, Subject: "Välkommen till Kimmunity!", Body: ""}
	mail.Body += "<html><head><meta http-equiv=\"Content-Type\" content=\"text/html; charset=utf-8\"></head>"
	mail.Body += "<body>"
	mail.Body += fmt.Sprintf("<h1>Välkommen till Kimmunity, %s!</h1>", username)
	mail.Body += fmt.Sprintf("<p><b><a href=\"https://www.kimmunity.se/set-password/%s\">Klicka här för att aktivera ditt konto!</a></b></p>", hexstring)
	mail.Body += "<p>Om du inte kan klicka på länken, kopiera länken nedan och klistra in den i din webbläsares addressfält:<br>"
	mail.Body += fmt.Sprintf("https://www.kimmunity.se/set-password/%s</p>", hexstring)
	mail.Body += "<p>Länken slutar att fungera en timme efter att detta mail skickades, om tiden går ut kan du begära en lösenordsåterställning.</p>"
	mail.Body += "<h2>Var det inte du?</h2>"
	mail.Body += "<p><a href=\"mailto:abuse@kimmunity.se\">Kontakta oss om du vill att vi tar bort kontot</a>, annars behöver du inte göra något.</p>"
	mail.Body += "</body></html>"

	mail.AddRecipient(userEmail)

	err = mail.Send()

	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "notification")
	}
}

func resetPassword(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	login := strings.ToLower(r.FormValue("login"))

	rn, _ := rand.Prime(rand.Reader, 64)
	hashsum := sha256.Sum256([]byte(fmt.Sprintf("%d%d", time.Now().UTC().UnixNano(), rn)))
	hexstring := hex.EncodeToString(hashsum[:])

	row := db.QueryRow("SELECT username, email FROM users WHERE username = $1 OR email = $1", login)
	var username string
	var userEmail string
	if err := row.Scan(&username, &userEmail); err != nil {
		//the user most likely does not exist
		log.Println(err)
		time.Sleep(time.Second * 1) //simulate sending an e-mail
		return
	}

	_, err := db.Exec("INSERT INTO account_keys(username, key) VALUES($1, $2)", username, hexstring)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	mail := email{SenderAddress: settings.Email.Username, Subject: "Återställ ditt lösenord", Body: ""}
	mail.Body += "<html><head><meta http-equiv=\"Content-Type\" content=\"text/html; charset=UTF-8\"><meta charset= \"UTF-8\"></head>"
	mail.Body += "<body>"
	mail.Body += "<h1>En lösenordsåterställning kommer lastad</h1>"
	mail.Body += fmt.Sprintf("<p><b><a href=\"https://www.kimmunity.se/set-password/%s\">Klicka här för att byta lösenord!</a></b></p>", hexstring)
	mail.Body += "<p>Om du inte kan klicka på länken, kopiera länken nedan och klistra in den i din webbläsares addressfält:<br>"
	mail.Body += fmt.Sprintf("https://www.kimmunity.se/set-password/%s</p>", hexstring)
	mail.Body += "<p>Länken slutar att fungera en timme efter att detta mail skickades, om tiden går ut kan du begära en ny lösenordsåterställning.</p>"
	mail.Body += "<h2>Var det inte du?</h2>"
	mail.Body += "<p>Då kan du sitta lugnt, ditt lösenord har inte ändrats.</p>"
	mail.Body += "</body></html>"

	mail.AddRecipient(userEmail)

	err = mail.Send()

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
	}
}

func setPassword(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	key := r.FormValue("key")
	password := r.FormValue("password")
	remember := false

	if r := r.FormValue("remember"); r == "true" {
		remember = true
	}

	var username string
	row := db.QueryRow("SELECT username FROM valid_account_keys WHERE key = $1", key)

	if err := row.Scan(&username); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "bad key")
		return
	}

	if len([]rune(password)) < 8 {
		w.WriteHeader(http.StatusNotAcceptable)
		fmt.Fprint(w, "short password")
		return
	}

	_, err := db.Exec("DELETE FROM account_keys WHERE key = $1", key)
	if err != nil {
		log.Println(err)
	}

	rawHash, err := bcrypt.GenerateFromPassword([]byte(password), 10)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	if _, err = db.Exec("UPDATE users SET pwhash = $1 WHERE username = $2", string(rawHash), username); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	cookie, err := persistSession(db, username, remember)

	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, cookie)
}

func persistSession(db *sql.DB, username string, remember bool) (*http.Cookie, error) {
	rn, _ := rand.Prime(rand.Reader, 64)
	hashsum := sha256.Sum256([]byte(fmt.Sprintf("%s%d%d", username, time.Now().UTC().UnixNano(), rn)))
	hexstring := hex.EncodeToString(hashsum[:])

	var t int64

	cookie := http.Cookie{Name: "KONVERGENS", Path: "/", Value: hexstring}

	if remember {
		row := db.QueryRow("INSERT INTO sessions(username, session_id, expires) VALUES($1, $2, (EXTRACT(epoch FROM (NOW() + '1 year'::INTERVAL) AT TIME ZONE 'UTC')*1000)::BIGINT) RETURNING expires", username, hexstring)

		if err := row.Scan(&t); err != nil {
			return nil, err
		}

		cookie.Expires = time.UnixMilli(t)
	} else {
		if _, err := db.Exec("INSERT INTO sessions(username, session_id) VALUES($1, $2)", username, hexstring); err != nil {
			return nil, err
		}
	}

	return &cookie, nil
}

func createSession(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	login := r.FormValue("login")
	password := r.FormValue("password")
	remember := false

	if r := r.FormValue("remember"); r == "true" {
		remember = true
	}

	if u := checkCredentials(db, login, password); u != nil {
		cookie, err := persistSession(db, u.Username, remember)

		if err != nil {
			log.Panicln(err)
		}

		http.SetCookie(w, cookie)
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "invalid login")
	}
}

func destroySession(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	if identifier := getSessionIdentifier(r); identifier != "" {
		if _, err := db.Exec("DELETE FROM sessions WHERE session_id = $1", identifier); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			cookie := http.Cookie{Name: "KONVERGENS", Path: "/"}
			http.SetCookie(w, &cookie)
		}
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func getUserFollowerCount(db *sql.DB, username string) (uint, error) {
	row := db.QueryRow("SELECT COUNT(*) FROM follows f WHERE f.follow = $1", username)
	var followers uint
	if err := row.Scan(&followers); err != nil {
		log.Println(err)
		return 0, err
	}

	return followers, nil
}

func followUser(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()
	username := mux.Vars(r)["username"]

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if u.Username == username {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	_, err := db.Exec("INSERT INTO follows(username, follow) VALUES($1, $2)", u.Username, username)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	followers, err := getUserFollowerCount(db, username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, `{"followers": %d}`, followers)
}

func unFollowUser(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()
	username := mux.Vars(r)["username"]

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	_, err := db.Exec("DELETE FROM follows WHERE username = $1 AND follow = $2", u.Username, username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	followers, err := getUserFollowerCount(db, username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, `{"followers": %d}`, followers)
}

func getUserInfo(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	username := mux.Vars(r)["username"]

	row := db.QueryRow("SELECT COALESCE(u.name, u.username), (SELECT COUNT(*) FROM follows f WHERE f.follow = u.username), (SELECT COUNT(*) FROM follows f WHERE f.username = u.username), (SELECT CASE COUNT(*) WHEN 0 THEN FALSE ELSE TRUE END FROM follows f WHERE f.follow = u.username AND f.username = $1) FROM users u WHERE username = $2", u.Username, username)
	var name string
	var follows uint
	var following uint
	var followedByMe bool
	if err := row.Scan(&name, &follows, &following, &followedByMe); err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	ui := userInfo{
		BaseInfo: user{
			Username: username,
			Name:     &name,
		},
		Followers:    follows,
		Following:    following,
		FollowedByMe: followedByMe,
		IsMe:         username == u.Username,
	}

	out, _ := json.Marshal(ui)
	w.Write(out)
}

func deleteAccount(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if u.ProfilePic {
		f := path.Join(settings.Storage, "images", fmt.Sprintf("%s.jpg", u.Username))
		if err := os.Remove(f); err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	err := bulkDeleteImageAssets(db, "SELECT filename FROM images i LEFT JOIN posts p ON p.id = i.post_id WHERE p.username = $1", u.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	_, err = db.Exec("DELETE FROM users WHERE username = $1", u.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}
}

func searchUsers(w http.ResponseWriter, r *http.Request) {
	db := dbConnection()
	defer db.Close()

	u := getCurrentUser(db, r)
	if u == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	query := r.FormValue("query")
	if query == "" {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	components := strings.Fields(query)
	query = fmt.Sprintf("%s:*", strings.Join(components, ":* & "))

	rows, err := db.Query("SELECT username, name, ts_rank_cd(tsv, q) AS rank FROM view_users, to_tsquery($1) q WHERE q @@ tsv ORDER BY rank DESC", query)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println(err)
		return
	}

	users := make([]user, 0)
	var username string
	var name *string
	var rank float64
	for rows.Next() {
		if err := rows.Scan(&username, &name, &rank); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Println(err)
			return
		}

		users = append(users, user{Name: name, Username: username})
	}

	out, _ := json.Marshal(users)
	w.Write(out)
}
