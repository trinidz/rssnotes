package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"time"
)

var validSchemas = []string{"https", "http"}

func UrlJoin(baseUrl string, elem ...string) (result string, err error) {
	u, err := url.Parse(baseUrl)
	if err != nil {
		return
	}

	if len(elem) > 0 {
		elem = append([]string{u.Path}, elem...)
		u.Path = path.Join(elem...)
	}

	return u.String(), nil
}

func IsValidHttpUrl(rawUrl string) bool {
	parsedUrl, err := url.ParseRequestURI(rawUrl)
	if parsedUrl == nil {
		return false
	}
	if err != nil || !slices.Contains(validSchemas, parsedUrl.Scheme) {
		return false
	}
	return true
}

func IconUrlExists(url string) bool {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[DEBUG] %v", err)
		return false
	}

	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()

	req = req.WithContext(ctx)

	client := http.DefaultClient

	res, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] %v", err)
		return false
	} else if res.StatusCode >= 300 {
		log.Printf("[ERROR] status code: %d", res.StatusCode)
		return false
	}

	return true
}

func GetRelayListFromFile(filePath string) []string {
	file, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read file: %s", err)
	}

	var relayList []string
	if err := json.Unmarshal(file, &relayList); err != nil {
		log.Fatalf("Failed to parse JSON: %s", err)
	}

	for i, relay := range relayList {
		relayList[i] = "wss://" + strings.TrimSpace(relay)
	}
	return relayList
}
