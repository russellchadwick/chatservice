package main

import (
	"os"
	"os/signal"
	"syscall"

	"crypto/tls"
	log "github.com/Sirupsen/logrus"
	"github.com/mattn/go-xmpp"
	"github.com/nu7hatch/gouuid"
	pb "github.com/russellchadwick/chatservice/proto"
	"github.com/russellchadwick/configurationservice"
	"github.com/russellchadwick/rpc"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"strings"
	"sync"
)

type server struct {
	xmppClient           *xmpp.Client
	receivedChatMessages chan pb.ChatMessage
	subscribedMutex      sync.RWMutex
	subscribed           map[string]chan pb.ChatMessage
}

// Send passes the message to XMPP
func (c *server) Send(ctx context.Context, in *pb.ChatMessage) (*pb.SendResponse, error) {
	log.WithField("user", in.User).WithField("message", in.Message).Info("Send")

	_, err := c.xmppClient.Send(xmpp.Chat{
		Remote: in.User,
		Type:   "chat",
		Text:   in.Message,
	})

	if err != nil {
		log.WithField("error", err).Warn("Unable to send to XMPP")
	}

	return &pb.SendResponse{}, nil
}

// Subscribe makes a consumer that gets a copy of all XMPP messages and relays them to the client
func (c *server) Subscribe(in *pb.SubscribeRequest, stream pb.Chat_SubscribeServer) error {
	uuid, err := uuid.NewV4()
	if err != nil {
		return err
	}
	log.WithField("uuid", uuid.String()).Info("Generated unique id")

	subscribeChan := make(chan pb.ChatMessage, 10)
	c.subscribedMutex.Lock()
	c.subscribed[uuid.String()] = subscribeChan
	c.subscribedMutex.Unlock()
	log.Info("Created channel")

	defer func() {
		c.subscribedMutex.Lock()
		close(c.subscribed[uuid.String()])
		delete(c.subscribed, uuid.String())
		c.subscribedMutex.Unlock()
	}()

	for chatMessage := range subscribeChan {
		err := stream.Send(&chatMessage)
		if err != nil {
			return err
		}
	}

	return nil
}

func newChatServer() *server {
	chatServer := server{}
	chatServer.subscribed = make(map[string]chan pb.ChatMessage)
	server, user, password := getConfiguration()
	chatServer.xmppClient = connectToXmpp(server, user, password)
	chatServer.receivedChatMessages = make(chan pb.ChatMessage, 10)
	go receiveXmpp(chatServer.xmppClient, chatServer.receivedChatMessages)
	go fanoutReceivedChats(&chatServer)
	return &chatServer
}

func getConfiguration() (server, user, password *string) {
	configurationClient := configurationservice.Client{}

	server, err := configurationClient.GetConfiguration("chat/server")
	if err != nil {
		log.WithField("error", err).Panicln("error retrieving config")
	}
	log.WithField("server", *server).Infoln("Got configuration")

	user, err = configurationClient.GetConfiguration("chat/user")
	if err != nil {
		log.WithField("error", err).Panicln("error retrieving config")
	}
	log.WithField("user", *user).Infoln("Got configuration")

	password, err = configurationClient.GetConfiguration("chat/password")
	if err != nil {
		log.WithField("error", err).Panicln("error retrieving config")
	}
	log.WithField("password", *password).Infoln("Got configuration")

	return server, user, password
}

func receiveXmpp(xmppClient *xmpp.Client, receivedChatMessages chan<- pb.ChatMessage) {
	for {
		chat, err := xmppClient.Recv()
		if err != nil {
			log.WithField("error", err).Fatal("XMPP receive error")
		}

		switch v := chat.(type) {
		case xmpp.Chat:
			chatMessage := pb.ChatMessage{
				User:    v.Remote,
				Message: v.Text,
			}

			if len(chatMessage.Message) > 0 {
				log.WithFields(log.Fields{
					"from":    chatMessage.User,
					"message": chatMessage.Message,
				}).Info("XMPP chat received")

				receivedChatMessages <- chatMessage
			}
		}
	}
}

func fanoutReceivedChats(chatServer *server) {
	for chatMessage := range chatServer.receivedChatMessages {
		chatServer.subscribedMutex.RLock()
		for id, subscriberChan := range chatServer.subscribed {
			log.WithField("id", id).Info("Faning out to subscribers")
			subscriberChan <- chatMessage
		}
		chatServer.subscribedMutex.RUnlock()
	}
}

func connectToXmpp(server, user, password *string) (xmppClient *xmpp.Client) {
	if server == nil {
		return nil
	}

	xmpp.DefaultConfig = tls.Config{
		ServerName:         serverName(*server),
		InsecureSkipVerify: true,
	}

	options := xmpp.Options{
		Host:          *server,
		User:          *user,
		Password:      *password,
		NoTLS:         false,
		Debug:         false,
		Session:       false,
		Status:        "xa",
		StatusMessage: "Xmppbot",
	}

	log.WithFields(log.Fields{
		"server":   *server,
		"user":     *user,
		"password": *password,
	}).Info("Connecting to XMPP")

	xmppClient, err := options.NewClient()
	if err != nil {
		log.WithField("error", err).Panic("XMPP connection error")
	}

	return xmppClient
}

func serverName(host string) string {
	return strings.Split(host, ":")[0]
}

func main() {
	rpcServer := rpc.Server{}
	go serve(&rpcServer)
	defer func() {
		err := rpcServer.Stop()
		if err != nil {
			log.WithField("error", err).Error("error during stop")
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	signal.Notify(signalChan, syscall.SIGTERM)
	<-signalChan
}

func serve(rpcServer *rpc.Server) {
	err := rpcServer.Serve("torrent", func(grpcServer *grpc.Server) {
		pb.RegisterChatServer(grpcServer, newChatServer())
	})
	if err != nil {
		log.WithField("error", err).Error("error from rpc serve")
	}
}
