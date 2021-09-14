package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var done chan interface{}
var interrupt chan os.Signal
var messages chan string
var broker_messages chan string

func reader() {
	for {
		message := <-messages
		fmt.Println("Message received: " + message)
	}
}

func getTopic(message string) string {
	data := map[string]interface{}{}
	json.Unmarshal([]byte(message), &data)

	return strings.Replace(data["asterisk_id"].(string), ":", "_", -1)
}

func getSystemName() string {
	uri := url.URL{
		Scheme: "http",
		Host:   os.Getenv("VOIP_HOST") + ":" + os.Getenv("VOIP_PORT"),
		Path:   "/ari/asterisk/info",
	}

	values := url.Values{}
	values.Add("api_key", os.Getenv("VOIP_USER")+":"+os.Getenv("VOIP_PASS"))
	values.Add("only", "config")
	uri.RawQuery = values.Encode()

	resp, err := http.Get(uri.String())

	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	data := map[string]interface{}{}
	json.Unmarshal(bodyBytes, &data)
	return data["config"].(map[string]interface{})["name"].(string)
}
func receiveHandler(connection *websocket.Conn) {
	defer close(done)
	for {
		_, msg, err := connection.ReadMessage()
		if err != nil {
			log.Println("Error in receive:", err)
			return
		}
		messages <- string(msg)
		broker_messages <- string(msg)
	}
}

func publishHandler(client MQTT.Client) {
	for {
		message := <-broker_messages
		// fmt.Println(getTopic(message))
		client.Publish("some_topic", 0, false, message)
	}
}

func getConnection() *websocket.Conn {
	uri := url.URL{
		Scheme: os.Getenv("VOIP_SCHEME"),
		Host:   os.Getenv("VOIP_HOST") + ":" + os.Getenv("VOIP_PORT"),
		Path:   os.Getenv("VOIP_PATH"),
	}

	values := url.Values{}
	values.Add("api_key", os.Getenv("VOIP_USER")+":"+os.Getenv("VOIP_PASS"))
	values.Add("app", os.Getenv("VOIP_APP"))
	values.Add("subscribeAll", "true")
	uri.RawQuery = values.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(uri.String(), nil)

	if err != nil {
		log.Fatal("Error connecting to websocket.", err)
	}
	return conn
}

func getBrokerClient() MQTT.Client {
	uri := url.URL{
		Scheme: os.Getenv("BROKER_SCHEME"),
		Host:   os.Getenv("BROKER_HOST") + ":" + os.Getenv("BROKER_PORT"),
		Path:   os.Getenv("BROKER_PATH"),
	}

	opts := MQTT.NewClientOptions().AddBroker(uri.String())
	opts.SetClientID("ARI-Handler")
	opts.SetUsername(os.Getenv("BROKER_USER"))
	opts.SetPassword(os.Getenv("BROKER_PASS"))

	client := MQTT.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	return client
}

func main() {
	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	done = make(chan interface{})
	interrupt = make(chan os.Signal)
	messages = make(chan string)
	broker_messages = make(chan string)

	signal.Notify(interrupt, os.Interrupt)
	// systemName := getSystemName()
	// fmt.Println(systemName)
	conn := getConnection()
	brokerClient := getBrokerClient()

	go reader()
	go receiveHandler(conn)
	go publishHandler(brokerClient)

	fmt.Println("* Running Application: " + os.Getenv("VOIP_APP"))
	fmt.Println("* Connected to VOIP: " + os.Getenv("VOIP_HOST") + ":" + os.Getenv("VOIP_PORT"))
	fmt.Println("* Connected to BROKER: " + os.Getenv("BROKER_HOST") + ":" + os.Getenv("BROKER_PORT"))

	for {
		select {
		case <-interrupt:
			log.Println("Received SIGINT interrupt signal.")
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("Error during closing websocket.", err)
				return
			}
		}
	}
}
