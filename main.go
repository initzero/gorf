package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"log"
)

import (
	mux "github.com/gorilla/mux"
	"rflib"
	"gorfirc"
)

type Page struct {
	Title string
	Body  []byte
}

func checkErr(err error, context string) {
	if err != nil {
		log.Printf("%s", context)
		log.Printf("%s", err)
	}
}

// render homepage
func Home(w http.ResponseWriter, r *http.Request) {
	title := "rf"
	p := &Page{Title: title, Body: []byte("rf")}
	t, err := template.ParseFiles("templates/home.html")
	if err != nil {
		log.Fatal(err)
	}

	// cache this html; if deploying something new, users will need to shift+f5 their browsers
	// to drop their local cache
	w.Header().Set("Cache-control", "private, max-age=5184000")

	t.Execute(w, p)
}

func rfServerHandler(w http.ResponseWriter, r *http.Request) {
	servers := rflib.RFServers()
	log.Printf("%#v", servers)
	encoder := json.NewEncoder(w)
	err := encoder.Encode(servers)
	checkErr(err, "json encode")
}

func main() {
	gorfirc.SetupIRC("irc.rizon.net:6667", "#ahnenerbe")
	r := mux.NewRouter()
	r.HandleFunc("/", Home)
	r.HandleFunc("/rf/servers", rfServerHandler)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.ListenAndServe(":8080", r)
}
