package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"rssnotes/metrics"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"github.com/nbd-wtf/go-nostr"
)

var (
	fp     = gofeed.NewParser()
	client = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return errors.New("stopped after 2 redirects")
			}
			return nil
		},
		Timeout: 5 * time.Second,
	}
)

var rss_types = []string{
	"rss+xml",
	"atom+xml",
	"feed+json",
	"text/xml",
	"application/xml",
}

func getFeedUrl(url string) string {
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return ""
	} else if resp.StatusCode >= 300 {
		log.Printf("[ERROR] status code: %d", resp.StatusCode)
		return ""
	}

	ct := resp.Header.Get("Content-Type")
	for _, typ := range rss_types {
		if strings.Contains(ct, typ) {
			return url
		}
	}

	if strings.Contains(ct, "text/html") {
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			log.Printf("[ERROR] %s", err)
			return ""
		}

		for _, typ := range rss_types {
			href, _ := doc.Find(fmt.Sprintf("link[type*='%s']", typ)).Attr("href")
			if href == "" {
				continue
			}
			if !strings.HasPrefix(href, "http") && !strings.HasPrefix(href, "https") {
				href, _ = UrlJoin(url, href)
			}
			return href
		}
	}

	return ""
}

func parseFeedForUrl(url string) (*gofeed.Feed, error) {
	//metrics.CacheMiss.Inc()
	fp.RSSTranslator = NewCustomTranslator()
	feed, err := fp.ParseURL(url)
	if err != nil {
		log.Print("[ERROR] ", err)
		return nil, err
	}

	// cleanup
	for i := range feed.Items {
		feed.Items[i].Content = ""
	}

	return feed, nil
}

func parseFeedForPubkey(pubKey string, deleteFailingFeeds bool) (*gofeed.Feed, Entity) {
	pubKey = strings.TrimSpace(pubKey)

	entity, err := getSavedEntity(pubKey)
	if err != nil {
		log.Printf("[ERROR] failed to retrieve entity with pubkey '%s': %v", pubKey, err)
		//metrics.AppErrors.With(prometheus.Labels{"type": "SQL_SCAN"}).Inc()
		return nil, entity
	}

	if !IsValidHttpUrl(entity.URL) {
		log.Printf("[INFO] invalid url %q", entity.URL)
		// if deleteFailingFeeds {
		// }
		return nil, entity
	}

	parsedFeed, err := parseFeedForUrl(entity.URL)
	if err != nil {
		log.Printf("[ERROR] failed to parse feed at url %q: %v", entity.URL, err)
		if deleteFailingFeeds {
			// TODO: think
			// if err := deleteEntityInBookmarkEvent(entity.PublicKey); err != nil {
			// 	log.Printf("[ERROR] could not delete feed '%q'...Error: %s ", entity.URL, err)
			// } else {
			// 	followAction := FollowManagment{
			// 		Action:       Delete,
			// 		FollowEntity: Entity{PublicKey: entity.PublicKey},
			// 	}
			//log.Printf("[INFO] Deleteing failed feed %s", entity.URL)
			// 	followManagmentCh <- followAction
			// }
		}
		return nil, entity
	}
	return parsedFeed, entity
}

func createMetadataNote(pubkey string, privkey string, feed *gofeed.Feed, profilePictureUrl string) error {

	if _, feedMetadata, _ := getLocalMetadataEvent(pubkey); feedMetadata.ID != "" {
		if time.Now().Unix()-feedMetadata.CreatedAt.Time().Unix() < int64(s.FeedMetadataRefreshDays*86400) {
			//log.Printf("[DEBUG] recent metadata exists at event ID %s created at: %v", feedMetadata.ID, feedMetadata.CreatedAt.Time().Unix())
			return nil
		}
	}

	var theDescription = feed.Description
	var theFeedTitle = feed.Title
	if strings.Contains(feed.Link, "reddit.com") {
		var subredditParsePart1 = strings.Split(feed.Link, "/r/")
		var subredditParsePart2 = strings.Split(subredditParsePart1[1], "/")
		theDescription = feed.Description + fmt.Sprintf(" #%s", subredditParsePart2[0])

		theFeedTitle = "/r/" + subredditParsePart2[0]
	}
	metadata := map[string]string{
		"name":  theFeedTitle + " (RSS Feed)",
		"about": theDescription + "\n\n" + feed.Link,
	}

	if profilePictureUrl != "" {
		metadata["picture"] = profilePictureUrl
	} else if feed.Image != nil {
		metadata["picture"] = feed.Image.URL
	} else {
		metadata["picture"] = s.DefaultProfilePicUrl
	}

	content, err := json.Marshal(metadata)
	if err != nil {
		log.Print("[ERROR] marshaling metadata content", err)
		return err
	}

	createdAt := nostr.Timestamp(time.Now().Unix())

	evt := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(createdAt),
		Kind:      nostr.KindProfileMetadata,
		Tags:      nostr.Tags{[]string{"proxy", feed.FeedLink, "rss"}},
		Content:   string(content),
	}
	evt.ID = string(evt.Serialize())

	if err := evt.Sign(privkey); err != nil {
		log.Print("[ERROR]", err)
		return err
	}

	relay.BroadcastEvent(&evt)

	for _, store := range relay.StoreEvent {
		store(context.TODO(), &evt)
	}

	metrics.KindProfileMetadataCreated.Inc()
	log.Printf("[DEBUG] metadata note for %s created with ID %s with createdat %d", feed.Link, evt.ID, evt.CreatedAt.Time().Unix())
	return nil
}

func feedItemToNote(pubkey string, item *gofeed.Item, feed *gofeed.Feed, defaultCreatedAt time.Time, _ string, maxContentLength int) nostr.Event {
	content := ""
	if item.Title != "" {
		content = "**" + item.Title + "**"
	}

	mdConverter := md.NewConverter("", true, nil)
	mdConverter.AddRules(GetConverterRules()...)

	description, err := mdConverter.ConvertString(item.Description)
	if err != nil {
		log.Printf("[WARN] failure to convert description to markdown (defaulting to plain text): %v", err)
		p := bluemonday.StripTagsPolicy()
		description = p.Sanitize(item.Description)
	}

	if !strings.EqualFold(item.Title, description) && !strings.Contains(feed.Link, "stacker.news") && !strings.Contains(feed.Link, "reddit.com") {
		content += "\n\n" + description
	}

	shouldUpgradeLinkSchema := false

	if strings.Contains(feed.Link, "reddit.com") {
		var subredditParsePart1 = strings.Split(feed.Link, "/r/")
		var subredditParsePart2 = strings.Split(subredditParsePart1[1], "/")
		var theHashtag = fmt.Sprintf(" #%s", subredditParsePart2[0])

		content = content + "\n\n" + theHashtag
	}

	content = html.UnescapeString(content)
	if len(content) > maxContentLength {
		content = content[0:(maxContentLength-1)] + "…"
	}

	if shouldUpgradeLinkSchema {
		item.Link = strings.ReplaceAll(item.Link, "http://", "https://")
	}

	// Handle comments
	if item.Custom != nil {
		if comments, ok := item.Custom["comments"]; ok {
			content += fmt.Sprintf("\n\nComments: %s", comments)
		}
	}

	content += "\n\n" + item.Link

	createdAt := defaultCreatedAt
	//log.Printf("[DEBUG] item %s defaultCreatedAt %v", item.Title, defaultCreatedAt.Unix())
	if item.UpdatedParsed != nil {
		createdAt = *item.UpdatedParsed
		//log.Printf("[DEBUG] item %s UpdatedParsed %v", item.Title, item.UpdatedParsed.Unix())
	}
	if item.PublishedParsed != nil {
		createdAt = *item.PublishedParsed
		//log.Printf("[DEBUG] item %s PublishedParsed %v", item.Title, item.PublishedParsed.Unix())
	}

	composedProxyLink := feed.FeedLink
	if item.GUID != "" {
		composedProxyLink += fmt.Sprintf("#%s", url.QueryEscape(item.GUID))
	}

	evt := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(createdAt.Unix()),
		Kind:      nostr.KindTextNote,
		Tags:      nostr.Tags{[]string{"proxy", composedProxyLink, "rss"}},
		Content:   strings.ToValidUTF8(content, ""),
	}
	evt.ID = string(evt.Serialize())

	return evt
}

func getPrivateKeyFromFeedUrl(url string, secret string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(url))
	r := m.Sum(nil)
	return hex.EncodeToString(r)
}

func checkAllFeeds() {
	newBookmarkCreated := false
	currentEntities, err := getSavedEntities()
	if err != nil {
		log.Print("[ERROR] could not retrieve entities")
		return
	}
	for _, currentEntity := range currentEntities {
		if !TimetoUpdateFeed(currentEntity) {
			//log.Printf("[DEBUG] not time to update %s. Time since last check: %d Avg post time: %d", currentEntity.URL, time.Now().Unix()-currentEntity.LastCheckedTime, currentEntity.AvgPostTime)
			continue
		}

		lastPostTime := int64(0)
		allPostTimes := make([]int64, 0)

		parsedFeed, entity := parseFeedForPubkey(currentEntity.PubKey, s.DeleteFailingFeeds)
		if parsedFeed == nil {
			continue
		}

		if err := createMetadataNote(currentEntity.PubKey, currentEntity.PrivateKey, parsedFeed, s.DefaultProfilePicUrl); err != nil {
			log.Printf("[ERROR] could not create metadata note: %s", err)
		}

		for _, item := range parsedFeed.Items {
			defaultCreatedAt := time.Unix(time.Now().Unix(), 0)
			evt := feedItemToNote(currentEntity.PubKey, item, parsedFeed, defaultCreatedAt, entity.URL, s.MaxContentLength)
			if entity.LastPostTime < evt.CreatedAt.Time().Unix() {
				if err := evt.Sign(entity.PrivateKey); err != nil {
					log.Printf("[ERROR] %s", err)
					continue
				}
				log.Printf("[DEBUG] feed entity %s note created with ID %s", entity.URL, evt.ID)

				relay.BroadcastEvent(&evt)

				for _, store := range relay.StoreEvent {
					store(context.TODO(), &evt)
				}

				metrics.KindTextNoteCreated.Inc()
			}

			if evt.CreatedAt.Time().Unix() > lastPostTime {
				lastPostTime = evt.CreatedAt.Time().Unix()
			}

			allPostTimes = append(allPostTimes, evt.CreatedAt.Time().Unix())
		}

		if err := updateEntityTimesInBookmarkEvent(Entity{
			PubKey:          entity.PubKey,
			LastPostTime:    lastPostTime,
			LastCheckedTime: time.Now().Unix(),
			AvgPostTime:     CalcAvgPostTime(allPostTimes),
		}); err != nil {
			log.Printf("[ERROR] feed entity %s not updated", entity.URL)
		} else {
			newBookmarkCreated = true
		}
	}
	if newBookmarkCreated {
		deleteOldKBookmarkEvents()
	}
}

func initFeed(pubkey string, privkey string, feedURL string, parsedFeed *gofeed.Feed) (int64, []int64) {
	var lastPostTime int64
	postTimes := make([]int64, 0)

	for _, item := range parsedFeed.Items {
		defaultCreatedAt := time.Unix(time.Now().Unix(), 0)
		evt := feedItemToNote(pubkey, item, parsedFeed, defaultCreatedAt, feedURL, s.MaxContentLength)
		if err := evt.Sign(privkey); err != nil {
			log.Printf("[ERROR] %s", err)
			continue
		}
		log.Printf("[DEBUG] feed entity %s note created with ID %s", feedURL, evt.ID)

		relay.BroadcastEvent(&evt)

		for _, store := range relay.StoreEvent {
			store(context.TODO(), &evt)
		}

		metrics.KindTextNoteCreated.Inc()

		if evt.CreatedAt.Time().Unix() > lastPostTime {
			lastPostTime = evt.CreatedAt.Time().Unix()
		}

		postTimes = append(postTimes, evt.CreatedAt.Time().Unix())
	}

	return lastPostTime, postTimes
}
