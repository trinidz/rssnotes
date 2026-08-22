package relays

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"rssnotes/internal/helpers"
	"rssnotes/internal/models"
	"rssnotes/metrics"
	"sort"
	"strings"
	"time"

	"github.com/jaytaylor/html2text"
	"github.com/mmcdole/gofeed"
	"github.com/nbd-wtf/go-nostr"
)

func ParseFeedForUrl(url string) (*gofeed.Feed, error) {
	//metrics.CacheMiss.Inc()

	fp := gofeed.NewParser()
	fp.RSSTranslator = helpers.NewCustomTranslator()

	feed, err := fp.ParseURL(url)
	if err != nil {
		log.Printf("[ERROR] %s for %s", err, url)
		return nil, err
	} else if feed == nil {
		log.Printf("[DEBUG] no parsed feed returned for %s", url)
		return nil, nil
	}

	// cleanup
	/* 	for i := range feed.Items {
		feed.Items[i].Content = ""
	} */

	return feed, nil
}

func parseFeedForPubkey(pubKey string, deleteFailingFeeds bool) (*gofeed.Feed, error) {
	pubKey = strings.TrimSpace(pubKey)

	entity, err := GetEntity(pubKey)
	if err != nil {
		log.Printf("[ERROR] failed to get entity with pubkey '%s': %v", pubKey, err)
		return nil, err
	}

	/* 	if !helpers.IsValidHttpUrl(entity.URL) {
		log.Printf("[INFO] invalid url %q", entity.URL)
		return nil, entity
	} */

	parsedFeed, err := ParseFeedForUrl(entity.FeedURL)
	if err != nil {
		log.Printf("[ERROR] failed to parse feed at url %q: %v", entity.FeedURL, err)
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
		return nil, err
	}
	return parsedFeed, nil
}

func CreateMetadataNote(pubkey string, privkey string, metaData models.Profile) (*nostr.Event, error) {

	/*
	   if feedMetadata, _ := getLocalMetadataEvent(pubkey); feedMetadata.ID != "" {
	   		if time.Now().Unix()-feedMetadata.CreatedAt.Time().Unix() < int64(s.FeedMetadataRefreshDays*86400) {
	   			//log.Printf("[DEBUG] recent metadata exists at event ID %s created at: %v", feedMetadata.ID, feedMetadata.CreatedAt.Time().Unix())
	   			return nil
	   		}
	   	}
	*/

	var theDescription = metaData.About
	var theFeedTitle = metaData.Name
	if strings.Contains(metaData.Website, "reddit.com") {
		var subredditParsePart1 = strings.Split(metaData.Website, "/r/")
		var subredditParsePart2 = strings.Split(subredditParsePart1[1], "/")
		theDescription = metaData.About + fmt.Sprintf(" #%s", subredditParsePart2[0])
		theFeedTitle = "/r/" + subredditParsePart2[0]
	}

	metaData.Name = theFeedTitle
	metaData.About = theDescription
	metaData.DisplayName = theFeedTitle + " (RSS Feed)"

	if metaData.Picture == "" {
		metaData.Picture = s.DefaultProfilePicUrl
	}

	content, err := json.Marshal(metaData)
	if err != nil {
		log.Print("[ERROR] marshaling metadata content", err)
		return nil, err
	}

	createdAt := nostr.Timestamp(time.Now().Unix())

	evt := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(createdAt),
		Kind:      nostr.KindProfileMetadata,
		Tags:      nostr.Tags{[]string{"proxy", metaData.Website, "rss"}},
		Content:   string(content),
	}
	evt.ID = string(evt.Serialize())

	if err := evt.Sign(privkey); err != nil {
		log.Print("[ERROR]", err)
		return nil, err
	}

	rly.BroadcastEvent(&evt)

	for _, store := range rly.StoreEvent {
		store(context.TODO(), &evt)
	}

	metrics.KindProfileMetadataCreated.Inc()
	log.Printf("[DEBUG] metadata note for %s created with ID %s with createdat %d", metaData.Name, evt.ID, evt.CreatedAt.Time().Unix())
	return &evt, nil
}

func UpdateMetadataNote(pubkeyhex string, nostrProfile models.Profile) (nostr.Event, error) {

	// verify profile data here
	// TODO

	content, err := json.Marshal(nostrProfile)
	if err != nil {
		log.Print("[ERROR] marshaling metadata content", err)
		return nostr.Event{}, err
	}

	entity, err := GetEntity(pubkeyhex)
	if err != nil {
		return nostr.Event{}, err
	}

	createdAt := nostr.Timestamp(time.Now().Unix())

	evt := nostr.Event{
		PubKey:    entity.PubKey,
		CreatedAt: nostr.Timestamp(createdAt),
		Kind:      nostr.KindProfileMetadata,
		Tags:      nostr.Tags{[]string{"proxy", entity.FeedURL, "rss"}},
		Content:   string(content),
	}
	evt.ID = string(evt.Serialize())

	if err := evt.Sign(entity.PrivateKey); err != nil {
		log.Print("[ERROR]", err)
		return nostr.Event{}, err
	}

	rly.BroadcastEvent(&evt)

	for _, store := range rly.StoreEvent {
		store(context.TODO(), &evt)
	}

	metrics.KindProfileMetadataCreated.Inc()
	log.Printf("[DEBUG] metadata note updated for %s created with ID %s with createdat %d", entity.FeedTitle, evt.ID, evt.CreatedAt.Time().Unix())
	return evt, nil
}

func feedItemToNote(pubkey string, item *gofeed.Item, feedlink string) nostr.Event {

	// fmt.Printf("\n[DEBUG]----- title ----:%s", item.Title)
	// fmt.Printf("\n[DEBUG] desc: %s", item.Description)
	// fmt.Printf("\n[DEBUG] content: %s", item.Content)
	// fmt.Printf("\n[DEBUG] link: %s", item.Link)

	content := ""
	if item.Title != "" {
		content = "**" + item.Title + "**"
	}

	txt, err := html2text.FromString(item.Content)
	if err == nil {
		content += "\n\n" + txt
		//fmt.Printf("\n[DEBUG] content:\n%s itemContent:\n%s ", content, item.Content)
	} else {
		log.Printf("[ERROR] %s", err)
		//return "", err
	}

	/* 	if maxContentLength > 0 && len(content) > maxContentLength {
		content = content[0:(maxContentLength-1)] + "…"
	} */

	shouldUpgradeLinkSchema := false

	if shouldUpgradeLinkSchema {
		item.Link = strings.ReplaceAll(item.Link, "http://", "https://")
	}

	content += "\n\n" + item.Link

	createdAt := time.Unix(time.Now().Unix(), 0)

	//log.Printf("[DEBUG] item %s defaultCreatedAt %v", item.Title, defaultCreatedAt.Unix())
	if item.UpdatedParsed != nil {
		createdAt = *item.UpdatedParsed
		//log.Printf("[DEBUG] item %s UpdatedParsed %v", item.Title, item.UpdatedParsed.Unix())
	}
	if item.PublishedParsed != nil {
		createdAt = *item.PublishedParsed
		//log.Printf("[DEBUG] item %s PublishedParsed %v", item.Title, item.PublishedParsed.Unix())
	}

	composedProxyLink := feedlink
	if item.GUID != "" {
		composedProxyLink += fmt.Sprintf("#%s", url.QueryEscape(item.GUID))
	}

	evt := nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(createdAt.Unix()),
		Kind:      nostr.KindTextNote,
		Tags:      nostr.Tags{{"proxy", composedProxyLink, "rss"}, {"t", "rssnotes"}},
		Content:   content,
	}
	evt.ID = string(evt.Serialize())

	return evt
}

func GetPrivateKeyFromFeedUrl(url string) string {
	m := hmac.New(sha256.New, []byte(s.RelayPrivkey))
	m.Write([]byte(url))
	r := m.Sum(nil)

	/* 	sk := hex.EncodeToString(r)
	   	pk, _ := nostr.GetPublicKey(sk)

	   	// Encode the private key to nsec format
	   	nsec, err := nip19.EncodePrivateKey(sk)
	   	if err != nil {
	   		panic(err)
	   	}

	   	// Encode the public key to npub format
	   	npub, _ := nip19.EncodePublicKey(pk)

	   	fmt.Println("New Private Key (Hex):", sk)
	   	fmt.Println("New Public Key (Hex):", pk)
	   	fmt.Println("Private Key (nsec):", nsec)
	   	fmt.Println("Public Key (npub):", npub) */

	return hex.EncodeToString(r)
}

func InitFeed(pubkey string, privkey string, feedURL string, parsedFeed *gofeed.Feed) (int64, []int64) {
	var lastPostTime int64
	postTimes := make([]int64, 0)

	for _, item := range parsedFeed.Items {
		evt := feedItemToNote(pubkey, item, parsedFeed.FeedLink)
		if err := evt.Sign(privkey); err != nil {
			log.Printf("[ERROR] %s", err)
			continue
		}
		log.Printf("[DEBUG] feed entity %s note created with ID %s", feedURL, evt.ID)

		rly.BroadcastEvent(&evt)

		for _, store := range rly.StoreEvent {
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

// returns the average seconds between posts for given feed
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

// TRUE if feed exists in bookmark event
func FeedExists(pubkeyHex, privKeyHex, feedUrl string) (bool, error) {

	if feedUrl == "" {
		log.Printf("[ERROR] feedURL is empty")
		return false, fmt.Errorf("feedURL is empty")
	}

	bookmarkEvent, err := getBookMarkEvent()
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return false, err
	}

	if bookmarkEvent == nil {
		log.Printf("[DEBUG] no bookmark found")
		return false, nil
	}

	bookMarkTags := bookmarkEvent.Tags.GetAll([]string{s.RsslayTagKey})
	for _, tag := range bookMarkTags {
		if strings.Contains(tag.Value(), pubkeyHex) || strings.Contains(tag.Value(), privKeyHex) || strings.Contains(tag.Value(), feedUrl) {
			log.Printf("[DEBUG] feedUrl %s already exists", feedUrl)
			return true, nil
		}
	}

	log.Printf("[DEBUG] feed %s does not exist", feedUrl)
	return false, nil
}

func CheckAllFeeds() {
	newBookmarkCreated := false
	currentEntities, err := GetEntities()
	if err != nil {
		log.Printf("[ERROR] get entities: %s", err)
		return
	}

	for _, currentEntity := range currentEntities {
		if !helpers.TimetoUpdateFeed(currentEntity) {
			//log.Printf("[DEBUG] not time to update %s. Time since last check: %d Avg post time: %d", currentEntity.URL, time.Now().Unix()-currentEntity.LastCheckedTime, currentEntity.AvgPostTime)
			continue
		}

		lastPostTime := int64(0)
		allPostTimes := make([]int64, 0)

		//parsedFeed, err := parseFeedForPubkey(currentEntity.PubKey, s.DeleteFailingFeeds)
		parsedFeed, err := ParseFeedForUrl(currentEntity.FeedURL)
		if parsedFeed == nil || err != nil {
			if err != nil {
				log.Printf("[ERROR] %s", err)
			}
			continue
		}

		if currentEntity.Blastr {
			if currentMetadataEvt, _ := GetLocalMetadataEvent(currentEntity.PubKey); currentMetadataEvt.ID != "" {
				refreshMetadataEvt := time.Now().Unix()-currentMetadataEvt.CreatedAt.Time().Unix() > int64(s.FeedMetadataRefreshDays*86400)
				if refreshMetadataEvt {
					var prof models.Profile
					if err := json.Unmarshal([]byte(currentMetadataEvt.Content), &prof); err != nil {
						log.Printf("[ERROR] blasting metadata: %s ", err)
					} else {
						if updatedMetaEvt, err := UpdateMetadataNote(currentEntity.PubKey, prof); err == nil {
							BlastNostrEventCh <- updatedMetaEvt
						}
					}

					if rlyListEvt, err := CreateRelayListEvent(currentEntity.PubKey, currentEntity.PrivateKey); err != nil {
						log.Print("[ERROR] blasting relay list ", err)
					} else {
						BlastNostrEventCh <- rlyListEvt
					}
				}
			}
		}

		for _, item := range parsedFeed.Items {
			evt := feedItemToNote(currentEntity.PubKey, item, parsedFeed.FeedLink)
			if currentEntity.LastPostTime < evt.CreatedAt.Time().Unix() {
				if err := evt.Sign(currentEntity.PrivateKey); err != nil {
					log.Printf("[ERROR] %s", err)
					continue
				}
				log.Printf("[DEBUG] feed entity %s note created with ID %s", currentEntity.FeedURL, evt.ID)

				rly.BroadcastEvent(&evt)

				for _, store := range rly.StoreEvent {
					store(context.TODO(), &evt)
				}

				metrics.KindTextNoteCreated.Inc()

				// publish note to other relays
				if currentEntity.Blastr {
					BlastNostrEventCh <- evt
				}
			}

			if evt.CreatedAt.Time().Unix() > lastPostTime {
				lastPostTime = evt.CreatedAt.Time().Unix()
			}

			allPostTimes = append(allPostTimes, evt.CreatedAt.Time().Unix())
		}

		if err := UpdateEntity(currentEntity.PubKey,
			models.WithLastPostTime(lastPostTime),
			models.WithLastCheckedTime(time.Now().Unix()),
			models.WithAvgPostTime(CalcAvgPostTime(allPostTimes))); err != nil {
			log.Printf("[ERROR] feed entity %s times not updated", currentEntity.FeedURL)
		} else {
			newBookmarkCreated = true
		}
	}
	if newBookmarkCreated {
		deleteOldKBookmarkEvents()
	}
}

/* func feedItemToNote_old(pubkey string, item *gofeed.Item, feed *gofeed.Feed, defaultCreatedAt time.Time, _ string, maxContentLength int) nostr.Event {
	content := ""
	if item.Title != "" {
		content = "**" + item.Title + "**"
	}

	mdConverter := md.NewConverter("", true, nil)
	mdConverter.AddRules(helpers.GetConverterRules()...)

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
} */
