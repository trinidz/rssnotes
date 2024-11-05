package main

import (
	"net/url"
	"path"
	"slices"
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
