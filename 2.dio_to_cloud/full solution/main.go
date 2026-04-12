package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"diofinal/pkg/protos"
)

const (
	url   = "https://demo-ingate.analytics.sentio.sick.com/platform/api/v1/ingest/260f1722-9840-4ecd-8e4e-3066ee9e9207/bulk"
	token = "DeviceApiKey eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIyNjBmMTcyMi05ODQwLTRlY2QtOGU0ZS0zMDY2ZWU5ZTkyMDciLCJqdGkiOiI5ZThjZDJkMi02ZWYxLTQ5M2ItOTk4My03MjZmNzA3MDQ2NmUiLCJ0aWQiOiI3ZGI3NzU4ZC00YzExLTQ5MWYtODlkOS1kNjk4NDEzN2NhYjMiLCJleHAiOjI3MjI2Nzc4NzcsImlzcyI6Imh0dHA6Ly9pb3QuaW8iLCJhdWQiOiJJb1QifQ.EwVztIix31kgGeYu34PAkipsEF6yi4QGz05zU55f6yU"
)

func setDIODHigh(client protos.DigitalIOClient) error {
	// First set the direction to OUT
	_, err := client.SetDirection(context.Background(), &protos.DigitalIOSetDirectionRequest{
		Name:      "DIO_D",
		Direction: protos.IODirection_OUT,
	})
	if err != nil {
		return err
	}
	// Then write HIGH
	_, err = client.Write(context.Background(), &protos.DigitalIOWriteRequest{
		Name:  "DIO_D",
		State: protos.IOState_HIGH,
	})
	return err
}

func sendToCloud(jsonData, key string) {
	var data interface{}
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		log.Printf("Error unmarshaling jsonData: %v", err)
		return
	}
	payload := []map[string]interface{}{
		{
			"data": []interface{}{data},
			"link": map[string]string{
				"key": key,
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	fmt.Printf("Sending payload: %s\n", string(payloadBytes))
	if err != nil {
		log.Printf("Error marshaling payload: %v", err)
		return
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		log.Printf("Unexpected status code: %d", resp.StatusCode)
	}
}

func readAndOutput(client protos.DigitalIOClient) map[string]int {
	// List devices
	resp, err := client.ListDevices(context.Background(), &emptypb.Empty{})
	if err != nil {
		log.Printf("Error listing devices: %v", err)
		return nil
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
	return states
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

	counter := 0
	for {
		states := readAndOutput(client)
		if states != nil {
			jsonData, err := json.Marshal(states)
			if err != nil {
				log.Printf("Error marshaling JSON: %v", err)
			} else {
				fmt.Println(string(jsonData))
				counter++
				if counter%10 == 0 {
					sendToCloud(string(jsonData), "dio")
				}
			}
		}
		time.Sleep(time.Second)
	}
}