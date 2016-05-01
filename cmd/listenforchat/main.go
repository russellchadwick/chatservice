package main

import (
	"github.com/russellchadwick/chatservice"
	pb "github.com/russellchadwick/chatservice/proto"
)

func main() {
	client := chatservice.Client{}
	chats := make(chan pb.ChatMessage, 10)
	err := client.Subscribe(chats)
	if err != nil {
		panic(err)
	}
}
