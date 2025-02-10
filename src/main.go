package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

type JsonRPC struct {
	Id      string `json:"id"`
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	Result  string `json"result"`
}

var oneshotToken string

func main() {
	var config Config = parseConfig()
	if !checkConnection(config) {
		log.Fatal("Could Not Connect!")
	}
	getOneshot(config)

}

func parseConfig() Config {
	file, err := os.Open("config.json")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// Decode the JSON data into the config struct
	decoder := json.NewDecoder(file)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		panic(err)
	}
	return config
}

func checkConnection(config Config) (ConnectionStatus bool) {
	res, err := http.Get(config.Server.Address)
	if err != nil {
		log.Fatal(err)
		return false
	}
	fmt.Printf("Got Response Code %d\n", res.StatusCode)
	if res.StatusCode == 200 {
		return true
	}

	return false
}

func getOneshot(config Config) {
	res, err := http.Get(config.Server.Address + "/access/oneshot_token")
	if err != nil {
		log.Fatalf("Could not get One Shot Token Error: ", err)
		return
	}
	defer res.Body.Close()

	var data map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&data) // Decode directly from the response body
	if err != nil {
		log.Fatalf("Error decoding JSON:", err)
		return
	}
	resultInterface, ok := data["result"]
	if !ok || resultInterface == nil {
		log.Fatal("No Token Found")
		return
	}

	oneshotToken := fmt.Sprintf("%v", resultInterface)
	fmt.Printf("Got token %s\n", oneshotToken)
	return

}
