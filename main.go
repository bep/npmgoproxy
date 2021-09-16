package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bep/npmgoproxy/npmgop"
)

func main() {
	server, err := npmgop.Start()
	if err != nil {
		log.Fatal("failed to start proxy server:", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	fmt.Println("npmgoproxy running ...")

	<-stop

	if err := server.Shutdown(); err != nil {
		log.Fatal(err)
	}
}
