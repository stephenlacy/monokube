package main

// https://gobyexample.com/http-servers

import (
	"fmt"
	"net/http"
)

func hello(w http.ResponseWriter, req *http.Request) {

	fmt.Fprintf(w, "hello\n")
}

func headers(w http.ResponseWriter, req *http.Request) {

	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}
}

func main() {

	http.HandleFunc("/", hello)
	http.HandleFunc("/headers", headers)

	fmt.Println("starting on port :3000")
	http.ListenAndServe(":3000", nil)
}
