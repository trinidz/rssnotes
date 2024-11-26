package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
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

func GetConverterRules() []md.Rule {
	return []md.Rule{
		{
			Filter: []string{"h1", "h2", "h3", "h4", "h5", "h6"},
			Replacement: func(content string, selection *goquery.Selection, opt *md.Options) *string {
				content = strings.TrimSpace(content)
				return md.String(content)
			},
		},
		{
			Filter: []string{"img"},
			AdvancedReplacement: func(content string, selec *goquery.Selection, opt *md.Options) (md.AdvancedResult, bool) {
				src := selec.AttrOr("src", "")
				src = strings.TrimSpace(src)
				if src == "" {
					return md.AdvancedResult{
						Markdown: "",
					}, false
				}

				src = opt.GetAbsoluteURL(selec, src, "")

				text := fmt.Sprintf("\n%s\n", src)
				return md.AdvancedResult{
					Markdown: text,
				}, false
			},
		},
		{
			Filter: []string{"a"},
			AdvancedReplacement: func(content string, selec *goquery.Selection, opt *md.Options) (md.AdvancedResult, bool) {
				// if there is no href, no link is used. So just return the content inside the link
				href, ok := selec.Attr("href")
				if !ok || strings.TrimSpace(href) == "" || strings.TrimSpace(href) == "#" {
					return md.AdvancedResult{
						Markdown: content,
					}, false
				}

				href = opt.GetAbsoluteURL(selec, href, "")

				// having multiline content inside a link is a bit tricky
				content = md.EscapeMultiLine(content)

				// if there is no link content (for example because it contains an svg)
				// the 'title' or 'aria-label' attribute is used instead.
				if strings.TrimSpace(content) == "" {
					content = selec.AttrOr("title", selec.AttrOr("aria-label", ""))
				}

				// a link without text won't de displayed anyway
				if content == "" {
					return md.AdvancedResult{
						Markdown: "",
					}, false
				}

				replacement := fmt.Sprintf("%s (%s)", content, href)

				return md.AdvancedResult{
					Markdown: replacement,
				}, false
			},
		},
	}
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
