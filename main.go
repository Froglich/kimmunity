package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/gorilla/mux"
	"github.com/microcosm-cc/bluemonday"
)

type konfig struct {
	LogDir   string `json:"log_dir"`
	ConfDir  string `json:"conf_dir"`
	Storage  string `json:"storage"`
	Port     uint   `json:"port"`
	Client   string `json:"client"`
	Database struct {
		Host     string `json:"host"`
		Port     uint   `json:"port"`
		DBName   string `json:"dbname"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"database"`
	Email struct {
		Server   string `json:"server"`
		Port     uint   `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"email"`
}

var settings konfig

var sanitizer *bluemonday.Policy

const majorVersion uint = 1
const minorVersion uint = 0

func main() {
	fmt.Printf("Welcome to Kimmunity version %d.%d!\n\n", majorVersion, minorVersion)
	fmt.Println("Copyright (C) 2022 Kim Lindgren")
	fmt.Println("This program comes with ABSOLUTELY NO WARRANTY. This is free software,")
	fmt.Println("and you are welcome to redistribute it under certain conditions; visit")
	fmt.Println("https://www.gnu.org/licenses/gpl-3.0.en.html for details.)")
	fmt.Println("")

	sanitizer = bluemonday.UGCPolicy()

	log.Println("Reading configuration file...")
	content, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Panicln(err)
	}
	if err := json.Unmarshal(content[:], &settings); err != nil {
		log.Panicln(err)
	} else if settings.Database.Host == "" ||
		settings.Database.Port == 0 ||
		settings.Database.Username == "" ||
		settings.Database.Password == "" ||
		settings.Database.DBName == "" ||
		settings.Email.Server == "" ||
		settings.Email.Port == 0 ||
		settings.Email.Username == "" ||
		settings.Email.Password == "" {
		log.Panicln("config is incomplete")
	}

	f, err := os.OpenFile(path.Join(settings.LogDir, "log.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0655)
	if err != nil {
		log.Panicln(err)
	}
	defer f.Close()
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)

	db := dbConnection()
	validateAndInitializeDB(db)
	db.Close()

	log.Println("Setting up handlers...")
	r := mux.NewRouter()

	r.HandleFunc("/backend/users", createAccount).Methods("POST")
	r.HandleFunc("/backend/users", deleteAccount).Methods("DELETE")
	r.HandleFunc("/backend/login", createSession).Methods("POST")
	r.HandleFunc("/logout", destroySession).Methods("GET")

	r.HandleFunc("/backend/posts", getPosts).Methods("GET")
	r.HandleFunc("/backend/posts", addNewPost).Methods("POST")
	r.HandleFunc("/backend/posts/{post_id:\\w{8}-\\w{4}-\\w{4}-\\w{4}-\\w{12}}", getPost).Methods("GET")
	r.HandleFunc("/backend/posts/{post_id:\\w{8}-\\w{4}-\\w{4}-\\w{4}-\\w{12}}", deletePost).Methods("DELETE")
	r.HandleFunc("/backend/posts/{post_id:\\w{8}-\\w{4}-\\w{4}-\\w{4}-\\w{12}}/comments", commentOnPost).Methods("POST")
	r.HandleFunc("/backend/posts/{post_id:\\w{8}-\\w{4}-\\w{4}-\\w{4}-\\w{12}}/comments/{id:[0-9]+}", deleteComment).Methods("DELETE")
	r.HandleFunc("/backend/posts/{post_id:\\w{8}-\\w{4}-\\w{4}-\\w{4}-\\w{12}}/likes", likePost).Methods("POST")
	r.HandleFunc("/backend/posts/{post_id:\\w{8}-\\w{4}-\\w{4}-\\w{4}-\\w{12}}/likes", unLikePost).Methods("DELETE")
	r.HandleFunc("/backend/user/password", setPassword).Methods("POST")
	r.HandleFunc("/backend/user/reset-password", resetPassword).Methods("POST")

	r.HandleFunc("/backend/summarize-url", getPageSummary).Methods("GET")

	r.HandleFunc("/backend/events", getEvents).Methods("GET")

	r.HandleFunc("/backend/users/{username:[a-zA-Z0-9\\_\\-]{3,}}", getUserInfo).Methods("GET")
	r.HandleFunc("/backend/users/{username:[a-zA-Z0-9\\_\\-]{3,}}/followers", followUser).Methods("POST")
	r.HandleFunc("/backend/users/{username:[a-zA-Z0-9\\_\\-]{3,}}/followers", unFollowUser).Methods("DELETE")
	r.HandleFunc("/backend/users/{username:[a-zA-Z0-9\\_\\-]{3,}}/profile-picture", getUserProfilePicture).Methods("GET")
	r.HandleFunc("/backend/users/{username:[a-zA-Z0-9\\_\\-]{3,}}/profile-picture", setUserProfilePicture).Methods("POST")
	r.HandleFunc("/backend/users/{username:[a-zA-Z0-9\\_\\-]{3,}}/profile-picture", delUserProfilePicture).Methods("DELETE")

	r.HandleFunc("/backend/search", searchUsers).Methods("POST")

	r.HandleFunc("/login", unauthenticatedView("login.html")).Methods("GET")
	r.HandleFunc("/privacy-policy", unauthenticatedView("privacy-policy.html")).Methods("GET")
	r.HandleFunc("/reset-password", unauthenticatedView("reset-password.html")).Methods("GET")
	r.HandleFunc("/set-password/{key:[a-z0-9]{64}}", unauthenticatedView("set-password.html")).Methods("GET")
	r.HandleFunc("/posts/{post_id:\\w{8}-\\w{4}-\\w{4}-\\w{4}-\\w{12}}", authenticatedView("post.html")).Methods("GET")
	r.HandleFunc("/users/{username:[a-zA-Z0-9\\_\\-]{3,}}", authenticatedView("user.html")).Methods("GET")
	r.HandleFunc("/youtube/{video:[A-Za-z0-9-]{11}}", authenticatedView("youtube.html")).Methods("GET")
	r.HandleFunc("/", authenticatedView("index.html")).Methods("GET")

	r.HandleFunc("/my-profile", myProfile).Methods("GET")
	r.HandleFunc("/my-profile-picture", myProfilePicture).Methods("GET")

	r.PathPrefix("/static").Handler(http.StripPrefix("/static", http.FileServer(http.Dir(path.Join(settings.ConfDir, "static")))))
	r.PathPrefix("/images").Handler(http.StripPrefix("/images", http.FileServer(http.Dir(path.Join(settings.Storage, "images")))))

	log.Printf("Listening for connections on '%s:%d'\n", settings.Client, settings.Port)

	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", settings.Client, settings.Port), r); err != nil {
		log.Panicln(err)
	}
}
