package chatservice

import (
	log "github.com/Sirupsen/logrus"
	pb "github.com/russellchadwick/chatservice/proto"
	"github.com/russellchadwick/rpc"
	"golang.org/x/net/context"
	"io"
	"time"
)

// Client is used to speak with the chat service via rpc
type Client struct{}

// Send will relay the message to the user
func (c *Client) Send(user, message string) error {
	log.Debug("-> client.Send")
	start := time.Now()

	client := rpc.Client{}
	clientConn, err := client.Dial("chat")
	if err != nil {
		log.WithField("error", err).Error("error during dial")
		return err
	}
	defer func() {
		closeErr := clientConn.Close()
		if closeErr != nil {
			log.WithField("error", closeErr).Error("error during close")
		}
	}()

	grpcClient := pb.NewChatClient(clientConn)

	_, err = grpcClient.Send(context.Background(), &pb.ChatMessage{
		User:    user,
		Message: message,
	})
	if err != nil {
		log.WithField("error", err).Error("error from rpc")
		return err
	}

	elapsed := float64(time.Since(start)) / float64(time.Microsecond)
	log.WithField("elapsed", elapsed).Debug("<- client.Send")

	return nil
}

// Subscribe blocks and writes messages received on the channel
func (c *Client) Subscribe(chatMessagesReceived chan<- pb.ChatMessage) error {
	log.Debug("-> client.Subscribe")
	start := time.Now()

	client := rpc.Client{}
	clientConn, err := client.Dial("chat")
	if err != nil {
		log.WithField("error", err).Error("error during dial")
		return err
	}
	defer func() {
		closeErr := clientConn.Close()
		if closeErr != nil {
			log.WithField("error", closeErr).Error("error during close")
		}
	}()

	grpcClient := pb.NewChatClient(clientConn)

	stream, err := grpcClient.Subscribe(context.Background(), &pb.SubscribeRequest{})
	if err != nil {
		log.WithField("error", err).Error("error from rpc")
		return err
	}

	for {
		chatMessage, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		log.WithField("user", chatMessage.User).WithField("message", chatMessage.Message).Info("Received chat")
		chatMessagesReceived <- *chatMessage
	}

	elapsed := float64(time.Since(start)) / float64(time.Microsecond)
	log.WithField("elapsed", elapsed).Debug("<- client.Subscribe")

	return nil
}
