package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const (
	ipCheckAPI    = "https://api.ipify.org"
	cloudflareAPI = "https://api.cloudflare.com/client/v4"
)

var lastIP string

type UpdateDNSRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type CloudflareResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func main() {
	godotenv.Load(".env")

	intervalMinutes, err := strconv.Atoi(os.Getenv("INTERVAL_MINUTES"))
	if err != nil {
		log.Printf("Invalid INTERVAL_MINUTES value: %v\n", err)
		log.Println("Using default interval of 10 minutes.")
		intervalMinutes = 10
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	run()

	tc := time.Tick(time.Duration(intervalMinutes) * time.Minute)

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down gracefully...")
			return
		case <-tc:
			run()
		}
	}
}

func run() {
	currentIP, err := getCurrentIP()
	if err != nil {
		log.Printf("Error checking current IP: %v\n", err)
		return
	}

	log.Printf("Current IP: %s\n", currentIP)

	if lastIP == currentIP {
		log.Println("IP has not changed, no update needed.")
		return
	}

	if err = updateCloudflareRecord(currentIP); err != nil {
		log.Printf("Failed to update Cloudflare record: %v\n", err)
		return
	}

	lastIP = currentIP

	log.Printf("Successfully updated Cloudflare DNS record to %s\n", currentIP)
}

func getCurrentIP() (string, error) {
	resp, err := http.Get(ipCheckAPI)
	if err != nil {
		return "", fmt.Errorf("failed to get current IP: %w", err)
	}
	defer resp.Body.Close()
	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read IP response: %w", err)
	}
	return string(ip), nil
}

func updateCloudflareRecord(ip string) error {
	apiToken := os.Getenv("CF_API_TOKEN")
	zoneID := os.Getenv("CF_ZONE_ID")
	recordID := os.Getenv("CF_RECORD_ID")
	recordName := os.Getenv("CF_RECORD_NAME")

	if apiToken == "" || zoneID == "" || recordID == "" || recordName == "" {
		return fmt.Errorf("missing one or more required environment variables")
	}

	reqBody := UpdateDNSRequest{
		Type:    "A",
		Name:    recordName,
		Content: ip,
		TTL:     1,
		Proxied: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal update request: %w", err)
	}

	req, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareAPI, zoneID, recordID),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send update request: %w", err)
	}
	defer resp.Body.Close()

	var cfResp CloudflareResponse
	if err := json.NewDecoder(resp.Body).Decode(&cfResp); err != nil {
		return fmt.Errorf("failed to parse Cloudflare response: %w", err)
	}

	if !cfResp.Success {
		return fmt.Errorf("Cloudflare API returned error: %+v", cfResp.Errors)
	}

	return nil
}
