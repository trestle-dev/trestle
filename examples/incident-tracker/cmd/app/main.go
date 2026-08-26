package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

//go:embed index.html
var assets embed.FS

func main() {
	listen := flag.String("listen", "127.0.0.1:8091", "example application address")
	backend := flag.String("trestle", "http://127.0.0.1:8090", "Trestle base URL")
	flag.Parse()
	target, err := url.Parse(*backend)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page, _ := assets.ReadFile("index.html")
		_, _ = w.Write(page)
	})
	log.Printf("incident tracker listening on http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
