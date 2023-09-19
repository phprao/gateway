package main

import (
	"log"
	"net/http"
	"time"
	"voteapi/app/gateway/plugin"
)

func main() {

	log.Println("listen at : 8888")

	server := &http.Server{Addr: ":8888", Handler: plugin.NewGateway()}
	server.ReadTimeout = 60 * time.Second
	server.WriteTimeout = 60 * time.Second
	if err := server.ListenAndServe(); err != nil {
		log.Fatalln("ListenAndServe:", err)
	}
}
