package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"
	"net"
	"net/http"
	"encoding/json"

	"github.com/goburrow/modbus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"leafproject/pkg/protos"
)

const (
	grpcServerAddr = "unix:///var/run/hal/hal.sock"
	modbusPort     = "/dev/ttyUSB0"
	modbusBaud     = 9600
	slaveID        = 254
	startAddr      = 0
	quantity       = 2
	url   = "https://demo-ingate.analytics.sentio.sick.com/platform/api/v1/ingest/260f1722-9840-4ecd-8e4e-3066ee9e9207/bulk"
	token = "DeviceApiKey eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIyNjBmMTcyMi05ODQwLTRlY2QtOGU0ZS0zMDY2ZWU5ZTkyMDciLCJqdGkiOiI5ZThjZDJkMi02ZWYxLTQ5M2ItOTk4My03MjZmNzA3MDQ2NmUiLCJ0aWQiOiI3ZGI3NzU4ZC00YzExLTQ5MWYtODlkOS1kNjk4NDEzN2NhYjMiLCJleHAiOjI3MjI2Nzc4NzcsImlzcyI6Imh0dHA6Ly9pb3QuaW8iLCJhdWQiOiJJb1QifQ.EwVztIix31kgGeYu34PAkipsEF6yi4QGz05zU55f6yU"
)


type Data struct {
	Leaftemp     float64 `json:"leaftemp"`
	Leafhumidity float64 `json:"leafhumidity"`
	Mpbtemp      float64 `json:"mpbtemp"`
}

var ledClient protos.LedControlClient

func init() {
	conn, err := grpc.NewClient(grpcServerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	ledClient = protos.NewLedControlClient(conn)
}

func setLED(led string, state int) {
	var s bool
	if state == 1 {
		s = true
	} else {
		s = false
	}
	_, err := ledClient.SetState(context.Background(), &protos.SetStateRequest{
		LedName: led,
		State:   s,
	})
	if err != nil {
		log.Printf("Failed to set LED %s to %d: %v", led, state, err)
	}
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

func readSensor() (humidity, temperature float64, err error) {
	handler := modbus.NewRTUClientHandler(modbusPort)
	handler.BaudRate = modbusBaud
	handler.DataBits = 8
	handler.Parity = "N"
	handler.StopBits = 1
	handler.SlaveId = slaveID
	handler.Timeout = 2 * time.Second
	defer handler.Close()

	client := modbus.NewClient(handler)
	results, err := client.ReadHoldingRegisters(uint16(startAddr), uint16(quantity))
	if err != nil {
		return 0, 0, err
	}
	if len(results) < 2 {
		return 0, 0, fmt.Errorf("not enough data")
	}

	humidity = (float64(results[0])*255 + float64(results[1])) / 10.0
	temperature = (float64(results[2])*255 + float64(results[3])) / 10.0

	return humidity, temperature, nil
}

func queryMPB10IOLinkTemperature() float64 {
	socketPath := "/run/iolink/iolink-api.sock"
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://unix/iolink/v1/devices/master1port1/parameters/4352/value?format=iodd", nil)
	if err != nil {
		log.Println("Failed to build IOLink request:", err)
		return 0
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Println("IOLink request failed:", err)
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("IOLink request failed: %s", resp.Status)
		return 0
	}

	var body map[string]struct {
		Value json.Number `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		log.Println("Failed to decode IOLink JSON:", err)
		return 0
	}

	field, ok := body["Current_temperature"]
	if !ok {
		log.Println("Current_temperature not found in response")
		return 0
	}

	temp, err := field.Value.Float64()
	if err != nil {
		log.Println("Failed to parse temperature:", err)
		return 0
	}

	return temp
}

func main() {
	counter := 0
	for {
		humidity, temperature, err := readSensor()
		if err != nil {
			log.Printf("Error reading sensor: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		fmt.Printf("Humidity: %.1f%%, Temperature: %.1f°C\n", humidity, temperature)
		iolinkTemp := queryMPB10IOLinkTemperature()
		fmt.Printf("IOLink MPB10 Temperature: %.1f°C\n", iolinkTemp)
		counter++
		if counter%10 == 0 {
			data := Data{Leaftemp: temperature, Leafhumidity: humidity, Mpbtemp: iolinkTemp}
			jsonData, err := json.Marshal(map[string]Data{"leaf": data})
			if err != nil {
				log.Printf("Failed to marshal JSON: %v", err)
			} else {
				sendToCloud(string(jsonData), "leaf")
			}
		}
		if humidity < 10 {
			setLED("FUNCT1", 0)
			setLED("FUNCT2", 0)
			setLED("FUNCT3", 0)
		} else if humidity < 40 {
			setLED("FUNCT1", 1)
			setLED("FUNCT2", 0)
			setLED("FUNCT3", 0)
		} else if humidity < 70 {
			setLED("FUNCT1", 1)
			setLED("FUNCT2", 1)
			setLED("FUNCT3", 0)
		} else {
			setLED("FUNCT1", 1)
			setLED("FUNCT2", 1)
			setLED("FUNCT3", 1)
		}

		time.Sleep(1 * time.Second) // Adjust as needed
	}
}