package main

import (
	"encoding/json"
	"fmt"
	"io"
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
			FilePath     string `json:"file_path"`     // Capital F
			Progress     int    `json:"progress"`      // Capital P
			IsActive     bool   `json:"is_active"`     // Capital I
			FilePosition int    `json:"file_position"` // Capital F
			FileSize     int    `json:"file_size"`     // Capital F
		} `json:"virtual_sdcard"` // Add JSON tag for nested struct
		PrintStats struct {
			Filename      string `json:"filename"`       // Capital F
			TotalDuration int    `json:"total_duration"` // Capital T
			PrintDuration int    `json:"print_duration"` // Capital P
			FilamentUsed  int    `json:"filament_used"`  // Capital F
			State         string `json:"state"`          // Capital S
			Message       string `json:"message"`        // Capital M
			Info          struct {
				TotalLayer   int `json:"total_layer"`   // Capital T
				CurrentLayer int `json:"current_layer"` // Capital C
			} `json:"info"` // Add JSON tag for nested struct
		} `json:"print_stats"` // Add JSON tag for nested struct
	} `json:"status"` // Add JSON tag for nested struct
}

func main() {

	var config Config = parseConfig()

	var printerStatus PrinterStatus = getStats(config)
	fmt.Println("printer status filename: ", printerStatus.Status.PrintStats.Filename)
	fmt.Println("Status State: ", printerStatus.Status.PrintStats.State)
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

func getStats(config Config) PrinterStatus {
	res, err := http.Get(config.Server.Address + "/printer/objects/query?webhooks&virtual_sdcard&print_stats")
	if err != nil {
		log.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode > 299 {
		log.Fatalf("Response failed with status code: %d and\nbody: %s\n", res.StatusCode, body)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s", body)

	var moonrakerResponse MoonrakerResponse // Use the wrapper struct
	err = json.Unmarshal(body, &moonrakerResponse)
	if err != nil {
		log.Printf("Decoding error: %v\nBody:%s\n", err, body)
	}

	printerStatus := moonrakerResponse.Result // Extract the PrinterStatus
	return printerStatus
}
