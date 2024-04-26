package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	redis_pub_sub "slack-poc/redis"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var clients = make(map[string]*websocket.Conn) // Map to store clients by their provided ID
var mu sync.Mutex                              // Mutex to synchronize access
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Adjust according to your CORS policy
	},
}

// CORS middleware that allows all cross-origin requests
func allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		log.Println("Handling .....")

		// Check if it's a preflight request
		if r.Method == "OPTIONS" {
			log.Println("Handling OPTIONS preflight request")
			w.WriteHeader(http.StatusOK)
			return
		}

		// Pass down the request to the next middleware (or final handler)
		next.ServeHTTP(w, r)
	})
}

var channelsMap = map[string][]string{
	"channel-1": []string{"client-1", "client-2"},
	"channel-2": []string{"client-3", "client-4"},
	"channel-3": []string{"client-1", "client-2", "client-3", "client-4"},
}

var clientChannelMap = map[string][]string{
	"client-1": []string{"channel-1"},
	"client-2": []string{"channel-1"},
	"client-3": []string{"channel-2"},
	"client-4": []string{"channel-2"},
}

var subscribedChannels = []string{}

// Define a struct to match the expected JSON request body
type MessageRequest struct {
	ClientID string `json:"clientId"`
	Message  string `json:"message"`
}

func contains(slice []string , val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}


// handleMessages processes messages received on the Redis channel
func handleMessages(ch <-chan *redis.Message) {
	for msg := range ch {
		log.Printf("Received from channel  %s Message: %s", msg.Channel, msg.Payload)
		var payload MessageRequest
		err := json.Unmarshal([]byte(msg.Payload), &payload)
		if err != nil {
			log.Printf("Error parsing message payload: %v", err)
			continue // Skip this message if parsing fails
		}
		channelClients, exists := channelsMap[msg.Channel]
		if exists {
			for _, clientId := range channelClients {
				if clientId != payload.ClientID {
					ws, exist := clients[clientId]
					if exist {
						err := ws.WriteMessage(websocket.TextMessage, []byte(msg.Payload))
						if err != nil {
							// Handle error, e.g., log it or manage a disconnect
							log.Printf("Error sending message: %v", err)
						}
					}
				}
			}
		}
	}
}

func subscribeToChannels(clientID string) {
	channels, exists := clientChannelMap[clientID]

	if exists {
		for index, value := range channels {
			if !contains(subscribedChannels, value) {
				log.Printf("index %s value  %s", string(index), value)
				channel := redis_pub_sub.SubscribeToChannel(value)
				log.Printf("clientId %s channel suscribed %s", clientID, value)
				subscribedChannels = append(subscribedChannels, value)
				go handleMessages(channel)
			}
			
		}
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["username"]

	// create ws connection
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	// save ws connection to clientId map
	mu.Lock()
	clients[clientID] = ws
	mu.Unlock()

	log.Printf("Client %s connected", clientID)

	// fetch channels for client
	subscribeToChannels(clientID)

	for {

		log.Printf("Message from %s", clientID)

		_, msg, err := ws.ReadMessage()

		log.Printf("Message from client %s", clientID)

		if err != nil {
			log.Printf("Error from client %s: %v", clientID, err)
			mu.Lock()
			delete(clients, clientID)
			mu.Unlock()
			break
		}
		fmt.Printf("Received from %s: %s\n", clientID, msg)
	}
}

// Handler function for the /send/message endpoint
func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method is not supported.", http.StatusMethodNotAllowed)
		return
	}

	// Decode the JSON body into the struct
	var msgReq MessageRequest
	err := json.NewDecoder(r.Body).Decode(&msgReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// You can now use msgReq.ClientID and msgReq.Message as needed
	fmt.Fprintf(w, "Received message from %s: %s", msgReq.ClientID, msgReq.Message)

	msgBytes, err := json.Marshal(msgReq)
	if err != nil {
		http.Error(w, "Error encoding message", http.StatusInternalServerError)
		return
	}

	clientId := msgReq.ClientID

	var channel string
	if clientId == "client-1" || clientId == "client-2" {
		channel = "channel-1"
	} else {
		channel = "channel-2"
	}

	redis_pub_sub.PublishToChannel(channel, msgBytes)

}

func main() {
	router := mux.NewRouter()
	// Initialize Redis
	redis_pub_sub.InitRedis()

	router.Use(allowCORS)

	// Setup WebSocket endpoint
	router.HandleFunc("/ws/{username}", handleConnections)

	// Setup HTTP endpoint for sending messages
	router.HandleFunc("/send/message", sendMessageHandler)

	// Start server
	log.Println("HTTP server started on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
