package main

import (
	"log"
	"net/http"
	"os"

	"jodata/internal/httpapi"
	"jodata/internal/store"
)

func main() {
	addr := os.Getenv("JODATA_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mem, err := store.NewFileStore(os.Getenv("JODATA_DATA_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	handler := httpapi.NewServer(mem)

	log.Printf("jodata listening on %s", addr)
	if err := http.ListenAndServe(addr, handler.Routes()); err != nil {
		log.Fatal(err)
	}
}
