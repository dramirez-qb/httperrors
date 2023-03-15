package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"text/template"

	"github.com/julienschmidt/httprouter"
)

type Track struct {
	Site  string
	Code  int
	Extra string
}

func withTracing(next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		defer log.Printf("[%s] %q", request.Method, request.URL.String())
		log.Printf("Tracing request for %s", request.RequestURI)
		next.ServeHTTP(response, request)
	}
}

func withLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		log.Printf("Logged connection from %s", request.RemoteAddr)
		next.ServeHTTP(response, request)
	}
}

func use(h http.HandlerFunc, middleware ...func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	for _, m := range middleware {
		h = m(h)
	}
	return recoverHandler(h)
}

func recoverHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %+v", err)
				http.Error(response, http.StatusText(500), 500)
			}
		}()
		next.ServeHTTP(response, request)
	}
}

func healthCheckHandler(response http.ResponseWriter, r *http.Request) {
	// A very simple health check.
	response.WriteHeader(http.StatusOK)
	response.Header().Set("Content-Type", "application/json")

	// In the future we could report back on the status of our DB, or our cache
	// (e.g. Redis) by performing a simple PING, and include them in the response.
	fmt.Fprintf(response, `{"alive": true}`)
}

func pongHandler(response http.ResponseWriter, request *http.Request) {
	fmt.Fprintf(response, "pong")
}

func helloHandler(response http.ResponseWriter, request *http.Request) {
	response.WriteHeader(http.StatusOK)
	response.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(response, `Hello World`)
}

func oldcustomErrorHandler(response http.ResponseWriter, request *http.Request) {

	if strings.HasPrefix(request.URL.Path, "/static") {
		localFS.ServeHTTP(response, request)
		return
	}
	if strings.HasPrefix(request.URL.Path, "/track") {
		page := strings.Split(request.URL.Path, "/")
		code, _ := strconv.Atoi(page[2])
		response.WriteHeader(code)
		track := Track{Site: request.Host, Code: code, Extra: page[3]}
		templates := template.Must(template.ParseFiles("templates/index.html"))
		if err := templates.ExecuteTemplate(response, "index.html", track); err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	track := Track{Site: request.Host, Code: 420}
	templates := template.Must(template.ParseFiles("templates/index.html"))
	if err := templates.ExecuteTemplate(response, "index.html", track); err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
	}
}

var (
	//go:embed static/* templates
	content   embed.FS
	gitCommit string
	localFS   http.Handler
)

func main() {
	version := *flag.Bool("version", false, "Version")
	port := *flag.String("port", "8080", "port to use")
	flag.Parse()
	if version {
		fmt.Printf("version: %s\n", gitCommit)
		return
	}
	localFS = http.FileServer(http.FS(content))
	mux := httprouter.New()
	mux.HandlerFunc("GET", "/", use(helloHandler, withLogging, withTracing))
	mux.HandlerFunc("GET", "/healthz", use(healthCheckHandler))
	mux.HandlerFunc("GET", "/ping", use(pongHandler, withLogging, withTracing))
	mux.HandlerFunc("GET", "/favicon.ico", use(pongHandler, withLogging, withTracing))

	mux.NotFound = use(oldcustomErrorHandler, withLogging, withTracing)
	log.Printf("starting listening on %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
