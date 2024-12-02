package worker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"rssnotes/parser"
	"rssnotes/scraper"

	"golang.org/x/net/html/charset"
)

type FeedSource struct {
	Title string `json:"title"`
	Url   string `json:"url"`
}

type DiscoverResult struct {
	Feed     *parser.Feed
	FeedLink string
	Sources  []FeedSource
}

func DiscoverFeed(candidateUrl string) (*DiscoverResult, error) {
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
	feed, err := parser.ParseAndFix(bytes.NewReader(body), candidateUrl, cs)
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
	for url, title := range scraper.FindFeeds(content, candidateUrl) {
		sources = append(sources, FeedSource{Title: title, Url: url})
	}
	switch {
	case len(sources) == 0:
		return nil, errors.New("No feeds found at the given url")
	case len(sources) == 1:
		if sources[0].Url == candidateUrl {
			return nil, errors.New("Recursion!")
		}
		return DiscoverFeed(sources[0].Url)
	}

	result.Sources = sources
	return result, nil
}

var emptyIcon = make([]byte, 0)
var imageTypes = map[string]bool{
	"image/x-icon": true,
	"image/png":    true,
	"image/jpeg":   true,
	"image/gif":    true,
}

func FindFavicon(siteUrl, feedUrl string) (*[]byte, error) {
	urls := make([]string, 0)

	favicon := func(link string) string {
		u, err := url.Parse(link)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%s://%s/favicon.ico", u.Scheme, u.Host)
	}

	if siteUrl != "" {
		if res, err := client.get(siteUrl); err == nil {
			defer res.Body.Close()
			if body, err := io.ReadAll(res.Body); err == nil {
				urls = append(urls, scraper.FindIcons(string(body), siteUrl)...)
				if c := favicon(siteUrl); c != "" {
					urls = append(urls, c)
				}
			}
		}
	}

	if c := favicon(feedUrl); c != "" {
		urls = append(urls, c)
	}

	for _, u := range urls {
		res, err := client.get(u)
		if err != nil {
			continue
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			continue
		}

		content, err := io.ReadAll(res.Body)
		if err != nil {
			continue
		}

		ctype := http.DetectContentType(content)
		if imageTypes[ctype] {
			return &content, nil
		}
	}
	return &emptyIcon, nil
}

func FindFaviconURL(siteUrl, feedUrl string) (string, error) {
	urls := make([]string, 0)

	favicon := func(link string) string {
		u, err := url.Parse(link)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%s://%s/favicon.ico", u.Scheme, u.Host)
	}

	if siteUrl != "" {
		if res, err := client.get(siteUrl); err == nil {
			defer res.Body.Close()
			if body, err := io.ReadAll(res.Body); err == nil {
				urls = append(urls, scraper.FindIcons(string(body), siteUrl)...)
				if c := favicon(siteUrl); c != "" {
					urls = append(urls, c)
				}
			}
		}
	}

	if c := favicon(feedUrl); c != "" {
		urls = append(urls, c)
	}

	for _, u := range urls {
		res, err := client.get(u)
		if err != nil {
			continue
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			continue
		}

		content, err := io.ReadAll(res.Body)
		if err != nil {
			continue
		}

		ctype := http.DetectContentType(content)
		if imageTypes[ctype] {
			return u, nil
		}
	}
	return "", nil
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

func GetBody(url string) (string, error) {
	res, err := client.get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var r io.Reader

	ctype := res.Header.Get("Content-Type")
	if strings.Contains(ctype, "charset") {
		r, err = charset.NewReader(res.Body, ctype)
		if err != nil {
			return "", err
		}
	} else {
		r = res.Body
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
