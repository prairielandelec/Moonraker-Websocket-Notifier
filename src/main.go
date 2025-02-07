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

func main() {

	var config Config = parseConfig()

	getStats(config)
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

func getStats(config Config) {
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
}
