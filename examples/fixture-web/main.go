// fixture-web serves the synthetic M1 pages for manual CLI reproduction.
package main

import (
	"log"
	"net/http"
	"time"

	"ui-check/internal/demo"
)

func main() {
	server := &http.Server{
		Addr:              "127.0.0.1:8042",
		Handler:           http.HandlerFunc(demo.Handler),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Print("Fixture ready at http://127.0.0.1:8042/signup")
	log.Fatal(server.ListenAndServe())
}
