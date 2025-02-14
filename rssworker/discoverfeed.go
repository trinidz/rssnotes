package rssworker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"rssnotes/rssparser"
	"rssnotes/rssscraper"

	"golang.org/x/net/html/charset"
)

type FeedSource struct {
	Title string `json:"title"`
	Url   string `json:"url"`
}

type DiscoverResult struct {
	Feed     *rssparser.Feed
	FeedLink string
	Sources  []FeedSource
}

func DiscoverRssFeed(candidateUrl string) (*DiscoverResult, error) {
	result := &DiscoverResult{}
	// Query URL
	res, err := client.get(candidateUrl)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code %d", res.StatusCode)
	}
	cs := getCharset(res)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// Try to feed into parser
	feed, err := rssparser.ParseAndFix(bytes.NewReader(body), candidateUrl, cs)
	if err == nil {
		result.Feed = feed
		result.FeedLink = candidateUrl
		return result, nil
	}

	// Possibly an html link. Search for feed links
	content := string(body)
	if cs != "" {
		if r, err := charset.NewReaderLabel(cs, bytes.NewReader(body)); err == nil {
			if body, err := io.ReadAll(r); err == nil {
				content = string(body)
			}
		}
	}
	sources := make([]FeedSource, 0)
	for url, title := range rssscraper.FindFeeds(content, candidateUrl) {
		sources = append(sources, FeedSource{Title: title, Url: url})
	}
	switch {
	case len(sources) == 0:
		return nil, errors.New("No feeds found at the given url")
	case len(sources) == 1:
		if sources[0].Url == candidateUrl {
			return nil, errors.New("Recursion!")
		}
		return DiscoverRssFeed(sources[0].Url)
	}

	result.Sources = sources
	return result, nil
}

func getCharset(res *http.Response) string {
	contentType := res.Header.Get("Content-Type")
	if _, params, err := mime.ParseMediaType(contentType); err == nil {
		if cs, ok := params["charset"]; ok {
			if e, _ := charset.Lookup(cs); e != nil {
				return cs
			}
		}
	}
	return ""
}
