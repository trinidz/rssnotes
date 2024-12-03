package worker

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"rssnotes/scraper"
)

var imageTypes = map[string]bool{
	"image/x-icon": true,
	"image/png":    true,
	"image/jpeg":   true,
	"image/gif":    true,
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
