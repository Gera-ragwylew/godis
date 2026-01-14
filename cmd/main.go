package main

/*
#cgo LDFLAGS: -lportaudio -lwinmm -lole32 -lsetupapi -luuid -static
#cgo CFLAGS: -I/mingw64/include
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"godis/cmd/server"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const (
	configFileName = "config.json"
)

type ConfigJSON struct {
	IP         string  `json:"ip"`
	Port       string  `json:"port"`
	SampleRate float64 `json:"sampleRate"`
	FrameSize  int     `json:"frameSize"`
}

func main() {
	fmt.Println("=== GODIS RUNNING ===")
	defer fmt.Println("=== GODIS STOPPED ===")

	config, err := ReadConfigJSON(configFileName)
	if err != nil {
		log.Printf("Error reading configuration: %v\n", err)
		return
	}

	fmt.Printf("Configuration loaded:\n")
	fmt.Printf("IP: %v\n", config.IP)
	fmt.Printf("Port: %v\n", config.Port)
	fmt.Printf("SampleRate: %v\n", config.SampleRate)
	fmt.Printf("FrameSize: %v\n", config.FrameSize)

	server := server.NewServer(config)
	server.Start()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to stop")
	<-sigChan
	log.Println("Received interrupt signal")

	server.Stop()
}

func ReadConfigJSON(filename string) (*ConfigJSON, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config ConfigJSON
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%s file parsing error: %w", filename, err)
	}

	return &config, nil
}
