package main

import "log"

func main() {
	h, err := NewHttpServer()
	if err != nil {
		log.Printf("main: failed to create http server: %s", err)
	}

	log.Printf("main: listening on %s", h)

	h.Serve()
}
