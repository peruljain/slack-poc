package redis

import (
	"context"
	"log"

	"github.com/go-redis/redis/v8"
)

var rdb *redis.Client
var pubsub *redis.PubSub

func InitRedis() {
	// Initialize Redis client at package initialization.
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Redis server address
	})
	log.Println("redus server connected on :6379")
}

// SubscribeToChannel subscribes to a Redis channel and returns a channel to receive messages.
func SubscribeToChannel(channelName string) <-chan *redis.Message {
	ctx := context.Background() // Create a context

	// Subscribe to the desired channel.
	var err error
	pubsub = rdb.Subscribe(ctx, channelName)
	

	// Wait for the confirmation that subscription is created.
	_, err = pubsub.Receive(ctx)
	if err != nil {
		panic(err)
	}

	// Return the message channel.
	return pubsub.Channel()
}

func PublishToChannel(channelName string, message []byte) {
	ctx := context.Background()
	if _, err := rdb.Publish(ctx, channelName, message).Result(); err != nil {
		log.Printf("Failed to publish to channel %s: %v", channelName, err)
	} else {
		log.Printf("Message published to channel %s", channelName)
	}
}

// ClosePubSub closes the subscription.
func ClosePubSub() {
	if pubsub != nil {
		pubsub.Close()
	}
}
