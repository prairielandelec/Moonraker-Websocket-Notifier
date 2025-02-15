package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	Server struct {
		Port    int    `json:"port"`
		Address string `json:"address"`
	} `json:"server"`
}

type MoonrakerResponse struct {
	Result PrinterStatus `json:"result"` // Embed or include the PrinterStatus struct
}
type PrinterStatus struct {
	Eventtime string `json:"eventtime"` // Capital E, and add JSON tag
	Status    struct {
		Webhooks struct {
			State        string `json:"state"`         // Capital S
			StateMessage string `json:"state_message"` // Capital S
		} `json:"webhooks"` // Add JSON tag for nested struct
		VirtualSdcard struct {
			FilePath     string  `json:"file_path"`     // Capital F
			Progress     float32 `json:"progress"`      // Capital P
			IsActive     bool    `json:"is_active"`     // Capital I
			FilePosition int     `json:"file_position"` // Capital F
			FileSize     int     `json:"file_size"`     // Capital F
		} `json:"virtual_sdcard"` // Add JSON tag for nested struct
		PrintStats struct {
			Filename      string  `json:"filename"`       // Capital F
			TotalDuration float32 `json:"total_duration"` // Capital T
			PrintDuration float32 `json:"print_duration"` // Capital P
			FilamentUsed  int     `json:"filament_used"`  // Capital F
			State         string  `json:"state"`          // Capital S
			Message       string  `json:"message"`        // Capital M
			Info          struct {
				TotalLayer   int `json:"total_layer"`   // Capital T
				CurrentLayer int `json:"current_layer"` // Capital C
			} `json:"info"` // Add JSON tag for nested struct
		} `json:"print_stats"` // Add JSON tag for nested struct
	} `json:"status"` // Add JSON tag for nested struct
}

type JsonRPCRequest struct {
	Id      int    `json:"id"`
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
}
type JsonRPCRepsonse struct {
	Id      int    `json:"id"`
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	Result  string `json:"result"`
}
type JsonRPCNotify struct {
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	Result  string `json:"result"`
}
type IDOnly struct {
	ID *int `json:"id"`
}

var oneshotToken string

var requestCounter int = 1

func main() {
	var config Config = parseConfig()
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	if !checkConnection(config) {
		log.Fatal("Could Not Connect!")
	}
	gotOneshot := getOneshot(config)

	fmt.Printf("Oneshot: %v", gotOneshot)

	if gotOneshot {
		url := "ws://" + config.Server.Address + "/websocket?token=" + oneshotToken
		log.Printf("connecting to %s", url)

		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Fatal("dial:", err)
		}
		defer c.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				_, message, err := c.ReadMessage()
				if err != nil {
					log.Println("read:", err)
					return
				}
				//log.Printf("recv: %s", message)
				gotID := &IDOnly{}
				parseerr := json.Unmarshal(message, gotID)
				if parseerr != nil {
					log.Println("parse err", parseerr)
				}
				if gotID.ID != nil {
					log.Printf("got an ID of: %v\nContent:\n%s\n", *gotID.ID, message)
				} else {
					log.Println("ID field is missing or null in the JSON")
					// Do something else here
				}
			}
		}()
		command := &JsonRPCRequest{Id: requestCounter, Version: "2.0", Method: "server.info"}
		if wserr := c.WriteJSON(command); wserr != nil {
			log.Printf("Socket Send Error: %v", wserr)
		}

		for {
			select {
			case <-done:
				return
			case <-interrupt:
				log.Println("interrupt")

				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.
				err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				if err != nil {
					log.Println("write close:", err)
					return
				}
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				return
			}
		}
	}

}

func parseConfig() Config {
	file, err := os.Open("config.json")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		panic(err)
	}
	return config
}

func checkConnection(config Config) (ConnectionStatus bool) {
	res, err := http.Get("http://" + config.Server.Address)
	if err != nil {
		log.Fatal(err)
		return false
	}
	fmt.Printf("Got Response Code %d\n", res.StatusCode)

	return res.StatusCode == 200
}

func getOneshot(config Config) (rc bool) {
	res, err := http.Get("http://" + config.Server.Address + "/access/oneshot_token")
	if err != nil {
		log.Fatalf("Could not get One Shot Token Error: %v", err)
		return false
	}
	defer res.Body.Close()

	var data map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&data) // Decode directly from the response body
	if err != nil {
		log.Fatalf("Error decoding JSON: %v", err)
		return false
	}
	resultInterface, ok := data["result"]
	if !ok || resultInterface == nil {
		log.Fatal("No Token Found")
		return false
	}

	oneshotToken = fmt.Sprintf("%v", resultInterface)
	fmt.Printf("Got token %s\n", oneshotToken)
	return true
}
