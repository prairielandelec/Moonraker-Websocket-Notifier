package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
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
	Id      int             `json:"id"`
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
}
type JsonRPCNotify struct {
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	Result  string `json:"result"`
}
type IDOnly struct {
	ID *int `json:"id"`
}

type MethodOnly struct {
	Method string `json:"method"`
}

type KlippyConnectionStatus struct {
	Connected bool   `json:"klippy_connected"`
	State     string `json:"klippy_state"`
}

type PrinterStatusNotif struct {
	Connected            bool
	Layer                int
	TotalLayers          int
	TimeRemainingSeconds float64
	TimeRemaining        HHMMSS
	Progress             float32
	Filename             string
	State                string
}

type HHMMSS struct {
	Hours   int
	Minutes int
	Seconds int
}

type PrinterStatusJSON struct {
	Params []json.RawMessage `json:"params"`
}

type PrinterStatusParams struct {
	VirtualSdcard struct {
		Progress float32 `json:"progress"`  // Capital P
		IsActive bool    `json:"is_active"` // Capital I
	} `json:"virtual_sdcard"` // Add JSON tag for nested struct
	PrintStats struct {
		Filename      string  `json:"filename"`       // Capital F
		PrintDuration float64 `json:"print_duration"` // Capital P
		State         string  `json:"state"`          // Capital S
		Info          struct {
			TotalLayer   int `json:"total_layer"`   // Capital T
			CurrentLayer int `json:"current_layer"` // Capital C
		} `json:"info"` // Add JSON tag for nested struct
	} `json:"print_stats"` // Add JSON tag for nested struct
}

type PrinterStatusSubscribe struct {
	Id      int    `json:"id"`
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Objects struct {
			VirtualSdcard *string `json:"virtual_sdcard"` //need to be encoded as nulls so we use pointers
			PrintStats    *string `json:"print_stats"`
		} `json:"objects"`
	} `json:"params"`
}

var oneshotToken string

var requestCounter int = 1

var CurrentPrinterStatus PrinterStatusNotif

func main() {
	logFile, err := os.OpenFile("log.txt", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)
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
		id_message := make(chan json.RawMessage)
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
					//log.Printf("got an ID of: %v\nContent:\n%s\n", *gotID.ID, message)
					id_message <- message
				} else {
					//log.Println("ID field is missing or null in the JSON")
					noIdMethod := &MethodOnly{}
					hasmethod := json.Unmarshal(message, noIdMethod)
					if hasmethod != nil {
						log.Println("Parse Error, no method")
					}
					//log.Printf("\n\n%s\n\n", message)
					if noIdMethod.Method == "notify_status_update" {
						log.Printf("\n\nGot a Status Notify Update\n\n")
						ParseStatusNotif(message)
					}
					// Do something else here
				}
			}
		}()

		if getKlippyStatus(c, id_message, false) {
			log.Printf("we are connected\n")
			if subscribeToNotifs(c, id_message) {
				log.Println("Subscription Successfull. Ready to notify")
			}
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

func getKlippyStatus(c *websocket.Conn, id_message chan json.RawMessage, secondAttempt bool) (connected bool) {
	timeout := 5 * time.Second
	command := &JsonRPCRequest{Id: requestCounter, Version: "2.0", Method: "server.info"}
	currentCount := requestCounter
	requestCounter++
	if wserr := c.WriteJSON(command); wserr != nil {
		log.Printf("Socket Send Error: %v", wserr)
	}

	for {
		select {
		case data := <-id_message:
			jsonResponse := &JsonRPCRepsonse{}
			if statuserr := json.Unmarshal(data, jsonResponse); statuserr != nil {
				log.Println("Parsing of status failed: ", statuserr)
				return false
			}
			if currentCount == jsonResponse.Id {
				log.Printf("IDs Match!\nMethod:\n%v\nResponse:\n%s\n", jsonResponse.Method, jsonResponse.Result)
				klippyState := &KlippyConnectionStatus{}
				json.Unmarshal(jsonResponse.Result, klippyState)
				if klippyState.Connected && klippyState.State == "ready" {
					log.Println("Klippy Connected & Ready!")
					return true
				} else if klippyState.Connected && klippyState.State == "startup" && !secondAttempt {
					log.Println("Klippy in startup, waiting 10 secs & checking again")
					time.Sleep(10 * time.Second)
					if getKlippyStatus(c, id_message, true) {
						log.Println("Klippy Connected & Ready!")
						return true
					}
					log.Println("Klippy in startup for more than 10 seconds. Aborting")
					return false
				}
			}
		case <-time.After(timeout):
			log.Println("Status Timeout!")
			return false
		}
	}
}

func subscribeToNotifs(c *websocket.Conn, id_message chan json.RawMessage) (connected bool) {
	timeout := 5 * time.Second
	command := &PrinterStatusSubscribe{Id: requestCounter, Version: "2.0", Method: "printer.objects.subscribe"}
	currentCount := requestCounter
	requestCounter++

	jsonBytes, err := json.MarshalIndent(command, "", "    ") // Use MarshalIndent
	if err != nil {
		log.Printf("Error marshaling JSON: %v", err)
		// Handle the error appropriately (e.g., return, panic)
	} else {
		log.Printf("Sending JSON command:\n%s", jsonBytes) // Print the pretty-printed JSON
	}

	if wserr := c.WriteJSON(command); wserr != nil {
		log.Printf("Socket Send Error: %v", wserr)
	}

	for {
		select {
		case data := <-id_message:
			jsonResponse := &JsonRPCRepsonse{}
			if statuserr := json.Unmarshal(data, jsonResponse); statuserr != nil {
				log.Println("Parsing of status failed: ", statuserr)
				return false
			}
			if currentCount == jsonResponse.Id {
				log.Printf("IDs Match!\nMethod:\n%v\nResponse:\n%s\n", jsonResponse.Method, jsonResponse.Result)
				klippyState := &KlippyConnectionStatus{}
				json.Unmarshal(jsonResponse.Result, klippyState)
				return true
			}
		case <-time.After(timeout):
			log.Println("Status Timeout!")
			return false
		}
	}
}

func ParseStatusNotif(payload json.RawMessage) {
	parsedPayload := &PrinterStatusJSON{}

	parseerr := json.Unmarshal(payload, parsedPayload)

	if parseerr != nil {
		log.Println("parse err", parseerr)
	}
	if len(parsedPayload.Params) > 0 {
		parsedParams := &PrinterStatusParams{}

		paramseerr := json.Unmarshal(parsedPayload.Params[0], parsedParams) //The object we care about will always be index 0 according to Moonraker Docs. Index 2 is time rxd

		if paramseerr != nil {
			log.Println("params parse err", paramseerr)
		}
		if CurrentPrinterStatus.Layer != parsedParams.PrintStats.Info.CurrentLayer {
			CurrentPrinterStatus.Layer = parsedParams.PrintStats.Info.CurrentLayer
		}
		if CurrentPrinterStatus.TotalLayers != parsedParams.PrintStats.Info.TotalLayer {
			CurrentPrinterStatus.TotalLayers = parsedParams.PrintStats.Info.TotalLayer
		}
		if CurrentPrinterStatus.TimeRemainingSeconds != parsedParams.PrintStats.PrintDuration {
			CurrentPrinterStatus.TimeRemainingSeconds = parsedParams.PrintStats.PrintDuration
			CurrentPrinterStatus.TimeRemaining = FormatSecondsToHHMMSS(parsedParams.PrintStats.PrintDuration)
		}
		if CurrentPrinterStatus.State != parsedParams.PrintStats.State {
			CurrentPrinterStatus.State = parsedParams.PrintStats.State
		}
		if CurrentPrinterStatus.Progress != parsedParams.VirtualSdcard.Progress {
			CurrentPrinterStatus.Progress = parsedParams.VirtualSdcard.Progress
		}
		if CurrentPrinterStatus.Filename != parsedParams.PrintStats.Filename {
			CurrentPrinterStatus.Filename = parsedParams.PrintStats.Filename
		}
		log.Printf("Parsed Notif\nCurrent: %+v\nNew (should match)%+s", CurrentPrinterStatus, parsedPayload)
	}

}

func FormatSecondsToHHMMSS(seconds float64) HHMMSS {
	// Handle negative durations
	sign := 1
	if seconds < 0 {
		sign = -1
		seconds = math.Abs(seconds)
	}

	totalSeconds := int(seconds)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	secondsOnly := totalSeconds % 60

	return HHMMSS{
		Hours:   hours * sign, // Apply sign to hours
		Minutes: minutes,
		Seconds: secondsOnly,
	}
}
