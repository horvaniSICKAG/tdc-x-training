package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"diofinal/pkg/protos"
)

func setDIODHigh(client protos.DigitalIOClient) error {
	_, err := client.Write(context.Background(), &protos.DigitalIOWriteRequest{
		Name:  "DIO_D",
		State: protos.IOState_HIGH,
	})
	return err
}

func readAndOutput(client protos.DigitalIOClient) {
	// List devices
	resp, err := client.ListDevices(context.Background(), &emptypb.Empty{})
	if err != nil {
		log.Printf("Error listing devices: %v", err)
		return
	}

	// Read all other inputs and outputs
	states := make(map[string]int)
	for _, dev := range resp.Devices {
		if dev.Name == "DIO_D" {
			continue // Skip DIO_D
		}
		readResp, err := client.Read(context.Background(), &protos.DigitalIOReadRequest{Name: dev.Name})
		if err != nil {
			log.Printf("Error reading %s: %v", dev.Name, err)
			continue
		}
		if readResp.State == protos.IOState_HIGH {
			states[dev.Name] = 1
		} else {
			states[dev.Name] = 0
		}
	}

	// Output as JSON
	jsonData, err := json.Marshal(states)
	if err != nil {
		log.Printf("Error marshaling JSON: %v", err)
		return
	}
	fmt.Println(string(jsonData))
}

func main() {
	conn, err := grpc.Dial("unix:/var/run/hal/hal.sock", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := protos.NewDigitalIOClient(conn)

	// Set DIO_D to high at start
	if err := setDIODHigh(client); err != nil {
		log.Fatal(err)
	}

	for {
		readAndOutput(client)
		time.Sleep(time.Second)
	}
}