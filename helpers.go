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
	"sort"
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

func CalcAvgPostTime(feedPostTimes []int64) int64 {
	if len(feedPostTimes) < s.MinPostPeriodSamples {
		return int64(s.MaxAvgPostPeriodHrs * 60 * 60)
	}

	sort.SliceStable(feedPostTimes, func(i, j int) bool {
		return feedPostTimes[i] > feedPostTimes[j]
	})

	avgposttimesecs := (feedPostTimes[0] - feedPostTimes[len(feedPostTimes)-1]) / int64(len(feedPostTimes))

	if avgposttimesecs < int64(s.MinAvgPostPeriodMins*60) {
		return int64(s.MinAvgPostPeriodMins * 60)
	} else if avgposttimesecs > int64(s.MaxAvgPostPeriodHrs*60*60) {
		return int64(s.MaxAvgPostPeriodHrs * 60 * 60)
	}

	return avgposttimesecs
}

func TimetoUpdateFeed(rssfeed Entity) bool {
	return time.Now().Unix()-rssfeed.LastCheckedTime >= rssfeed.AvgPostTime
}
