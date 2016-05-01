package main

import (
	"flag"
	log "github.com/Sirupsen/logrus"
	"github.com/russellchadwick/chatservice"
)

func main() {
	user := flag.String("user", "", "User to send to")
	message := flag.String("message", "", "Message to send")
	flag.Parse()

	if *user == "" || *message == "" {
		flag.Usage()
		log.Error("Required input not present")
		return
	}

	client := chatservice.Client{}
	err := client.Send(*user, *message)
	if err != nil {
		log.WithField("error", err).Panic("error during send")
	}
}
