package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/gilliek/go-opml/opml"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/skip2/go-qrcode"

	"encoding/json"
	"log"
)

func handleFrontpage(w http.ResponseWriter, _ *http.Request) {

	items, err := getSavedEntries()
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	npub, _ := nip19.EncodePublicKey(s.RelayPubkey)
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/index.html", s.TemplatePath)))

	data := struct {
		RelayName        string
		RelayPubkey      string
		RelayNPubkey     string
		RelayDescription string
		RelayURL         string
		Count            int
		Entries          []Entry
	}{
		RelayName:        s.RelayName,
		RelayPubkey:      s.RelayPubkey,
		RelayNPubkey:     npub,
		RelayDescription: s.RelayDescription,
		RelayURL:         s.RelayURL,
		Count:            len(items),
		Entries:          items,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleCreateFeed(w http.ResponseWriter, r *http.Request, secret *string) {
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/created.html", s.TemplatePath)))
	//metrics.CreateRequests.Inc()
	entry := createFeed(r, secret)

	followAction := FollowManagment{
		Action: Sync,
	}
	followManagmentCh <- followAction

	data := struct {
		RelayName    string
		PubKey       string
		NPubKey      string
		Url          string
		ErrorCode    int
		Error        bool
		ErrorMessage string
	}{
		RelayName:    s.RelayName,
		PubKey:       entry.PubKey,
		NPubKey:      entry.NPubKey,
		Url:          entry.Url,
		ErrorCode:    entry.ErrorCode,
		Error:        entry.Error,
		ErrorMessage: entry.ErrorMessage,
	}

	err := tmpl.Execute(w, data)
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func createFeed(r *http.Request, secret *string) *Entry {
	urlParam := r.URL.Query().Get("url")

	entry := Entry{
		Error: false,
	}

	if !IsValidHttpUrl(urlParam) {
		log.Printf("[DEBUG] tried to create feed from invalid feed url '%q' skipping...", urlParam)
		entry.ErrorCode = http.StatusBadRequest
		entry.Error = true
		entry.ErrorMessage = "Invalid URL provided (must be in absolute format and with https or https scheme)..."
		return &entry
	}

	feedUrl := getFeedUrl(urlParam)
	if feedUrl == "" {
		entry.ErrorCode = http.StatusBadRequest
		entry.Error = true
		entry.ErrorMessage = "Could not find a feed URL in there..."
		return &entry
	}

	sk := getPrivateKeyFromFeedUrl(feedUrl, *secret)
	publicKey, err := nostr.GetPublicKey(sk)
	if err != nil {
		entry.ErrorCode = http.StatusInternalServerError
		entry.Error = true
		entry.ErrorMessage = "Bad private key: " + err.Error()
		log.Printf("[ERROR] bad private key from feed: %s", err)
		return &entry
	}

	publicKey = strings.TrimSpace(publicKey)

	if feedExists, err := feedExists(publicKey, sk, feedUrl); err != nil || feedExists {
		if feedExists {
			log.Printf("[DEBUG] feedUrl %s with pubkey %s already exists", feedUrl, publicKey)
			entry.ErrorMessage = fmt.Sprintf("Feed %s already exists", feedUrl)
		} else {
			log.Printf("[ERROR] could not determine if feedUrl %s with pubkey %s exists", feedUrl, publicKey)
			entry.ErrorMessage = fmt.Sprintf("Could not determine if feed %s exists", feedUrl)
		}
		entry.ErrorCode = http.StatusInternalServerError
		entry.Error = true
		return &entry
	}

	parsedFeed, err := parseFeedForUrl(feedUrl)
	if err != nil {
		entry.ErrorCode = http.StatusBadRequest
		entry.Error = true
		entry.ErrorMessage = "Can not parse feed: " + err.Error()
		log.Printf("[ERROR] can not parse feed %s", err)
		return &entry
	}

	if err := createMetadataNote(publicKey, sk, parsedFeed, s.DefaultProfilePicUrl); err != nil {
		log.Printf("[ERROR] creating metadata note %s", err)
	}

	latestCreatedAt := initFeed(publicKey, sk, feedUrl, parsedFeed)

	if err := addEntityToBookmarkEvent([]Entity{{publicKey, sk, feedUrl, latestCreatedAt}}); err != nil {
		log.Printf("[ERROR] feed entity %s not added to bookmark", feedUrl)
	}

	entry.Url = feedUrl
	entry.PubKey = publicKey
	entry.NPubKey, _ = nip19.EncodePublicKey(publicKey)

	if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", entry.NPubKey), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.QRCodePath, entry.NPubKey)); err != nil {
		log.Print("[ERROR]", err)
	}

	return &entry
}

func handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	feedPubkey := r.URL.Query().Get("pubkey")

	followAction := FollowManagment{
		Action:       Delete,
		FollowEntity: Entity{PublicKey: feedPubkey},
	}
	followManagmentCh <- followAction

	if err := deleteEntityInBookmarkEvent(feedPubkey); err != nil {
		log.Printf("[ERROR] could not delete feed '%q'...Error: %s ", feedPubkey, err)
	}

	items, err := getSavedEntries()
	if err != nil {
		log.Printf("[ERROR] %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	npub, _ := nip19.EncodePublicKey(s.RelayPubkey)
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/index.html", s.TemplatePath)))

	data := struct {
		RelayName        string
		RelayPubkey      string
		RelayNPubkey     string
		RelayDescription string
		RelayURL         string
		Count            int
		Entries          []Entry
	}{
		RelayName:        s.RelayName,
		RelayPubkey:      s.RelayPubkey,
		RelayNPubkey:     npub,
		RelayDescription: s.RelayDescription,
		RelayURL:         s.RelayURL,
		Count:            len(items),
		Entries:          items,
	}

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("[ERROR] %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleImportOpml(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/imported.html", s.TemplatePath)))

	type Results struct {
		RelayName    string
		Feeds        []*Entry
		GoodFeeds    int
		BadFeeds     int
		Error        bool
		ErrorMessage string
		ErrorCode    int
	}

	badResults := Results{
		RelayName:    s.RelayName,
		Feeds:        []*Entry{},
		GoodFeeds:    0,
		BadFeeds:     0,
		Error:        false,
		ErrorMessage: "OPML File Processed",
		ErrorCode:    0,
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		log.Printf("[ERROR] parse OPML file %s", err)
		badResults.ErrorMessage = "[ERROR] parse OPML file"
		if err := tmpl.Execute(w, badResults); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	file, _, err := r.FormFile("opml-file")
	if err != nil {
		log.Printf("[ERROR] form OPML file: %s", err)
		badResults.ErrorMessage = "[ERROR] formfile OPML"
		if err := tmpl.Execute(w, badResults); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Printf("[ERROR] reading OPML file: %s", err)
		badResults.ErrorMessage = "[ERROR] reading OPML file"
		if err := tmpl.Execute(w, badResults); err != nil {
			log.Print("[ERROR] ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	doc, err := opml.NewOPML(fileBytes)
	if err != nil {
		log.Printf("[ERROR] OPML bad file format %s", err)
		badResults.ErrorMessage = "[ERROR] OPML bad file format"
		if err := tmpl.Execute(w, badResults); err != nil {
			log.Print("[ERROR] ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	feedEntries := importFeeds(doc.Body.Outlines, &s.RandomSecret)
	numBadFeeds := 0

	for _, feed := range feedEntries {
		if feed.Error {
			numBadFeeds++
		}
	}

	results := Results{
		RelayName:    s.RelayName,
		Feeds:        feedEntries,
		GoodFeeds:    len(feedEntries) - numBadFeeds,
		BadFeeds:     numBadFeeds,
		Error:        false,
		ErrorMessage: "OPML File Processed",
		ErrorCode:    0,
	}

	if err := tmpl.Execute(w, results); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Print("[DEBUG] opml import started.")
}

func importFeeds(opmlUrls []opml.Outline, secret *string) []*Entry {

	feedEntries := make([]*Entry, 0)
	feedEntities := make([]Entity, 0)

	for _, urlParam := range opmlUrls {
		if !IsValidHttpUrl(urlParam.XMLURL) {
			feedEntries = append(feedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Invalid URL provided (must be in absolute format and with https or https scheme)...",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			log.Printf("[DEBUG] invalid feed url '%q' skipping...", urlParam)
			continue
		}

		feedUrl := getFeedUrl(urlParam.XMLURL)
		if feedUrl == "" {
			feedEntries = append(feedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Could not find a feed URL in there...",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			log.Printf("[DEBUG] Could not find a feed URL in %s", feedUrl)
			continue
		}

		sk := getPrivateKeyFromFeedUrl(feedUrl, *secret)
		publicKey, err := nostr.GetPublicKey(sk)
		if err != nil {
			feedEntries = append(feedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Bad private key: " + err.Error(),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			log.Printf("[ERROR] bad private key from feed: %s", err)
			continue
		}

		publicKey = strings.TrimSpace(publicKey)

		feedExists, err := feedExists(publicKey, sk, feedUrl)
		if feedExists {
			feedEntries = append(feedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: fmt.Sprintf("Feed %s already exists", feedUrl),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			log.Printf("[DEBUG] feedUrl %s with pubkey %s already exists", feedUrl, publicKey)
			continue
		} else if err != nil {
			feedEntries = append(feedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: fmt.Sprintf("Could not determine if feed %s exists", feedUrl),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			log.Printf("[ERROR] could not determine if feedUrl %s with pubkey %s exists", feedUrl, publicKey)
			continue
		}

		parsedFeed, err := parseFeedForUrl(feedUrl)
		if err != nil {
			feedEntries = append(feedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Can not parse feed: " + err.Error(),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			log.Printf("[ERROR] can not parse feed %s", err)
			continue
		}

		npub, _ := nip19.EncodePublicKey(publicKey)
		feedEntr := Entry{
			Url:          feedUrl,
			PubKey:       publicKey,
			NPubKey:      npub,
			ErrorMessage: "",
			Error:        false,
			ErrorCode:    0,
		}
		feedEntries = append(feedEntries, &feedEntr)

		go func() {
			if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", feedEntr.NPubKey), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.QRCodePath, feedEntr.NPubKey)); err != nil {
				log.Print("[ERROR] ", err)
			}

			if err := createMetadataNote(publicKey, sk, parsedFeed, s.DefaultProfilePicUrl); err != nil {
				log.Printf("[ERROR] creating metadata note %s", err)
			}
		}()

		latestCreatedAt := initFeed(publicKey, sk, feedUrl, parsedFeed)
		feedEntities = append(feedEntities, Entity{PublicKey: publicKey, PrivateKey: sk, URL: feedUrl, LastUpdate: latestCreatedAt})
	}

	if err := addEntityToBookmarkEvent(feedEntities); err != nil {
		log.Printf("[ERROR] adding feed entities: %s", err)
	}

	followAction := FollowManagment{
		Action: Sync,
	}
	followManagmentCh <- followAction

	return feedEntries
}

func handleExportOpml(w http.ResponseWriter, r *http.Request) {
	var rssOMPL = &opml.OPML{
		Version: "1.0",
		Head: opml.Head{
			Title:       "rsslay Feeds",
			DateCreated: time.Now().Format(time.RFC3339),
			OwnerName:   "rsslay",
		},
	}

	data, _ := getSavedEntities()

	for _, feed := range data {
		rssOMPL.Body.Outlines = append(rssOMPL.Body.Outlines, opml.Outline{
			Type:    "rss",
			Text:    feed.PublicKey,
			Title:   feed.PrivateKey,
			XMLURL:  feed.URL,
			HTMLURL: feed.URL,
			Created: strconv.FormatInt(feed.LastUpdate, 10),
		})
	}

	w.Header().Add("content-type", "application/opml")
	w.Header().Add("content-disposition", "attachment; filename="+time.Now().Format(time.DateOnly)+"-rsslay.opml")
	outp, err := rssOMPL.XML()
	if err != nil {
		log.Print("[ERROR] exporting opml file")
		http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
		return
	}

	fmt.Fprintf(w, "%s", outp)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/search.html", s.TemplatePath)))
	//metrics.SearchRequests.Inc()
	query := r.URL.Query().Get("query")
	if query == "" || len(query) <= 4 {

		errorData := struct {
			RelayName      string
			Count          uint64
			FilteredCount  uint64
			Entries        []Entry
			MainDomainName string
			Error          bool
			ErrorMessage   string
		}{
			RelayName:     s.RelayName,
			Count:         0,
			FilteredCount: 0,
			Entries:       nil,
			Error:         true,
			ErrorMessage:  "Please enter more than 5 characters to search!",
		}

		if err := tmpl.Execute(w, errorData); err != nil {
			log.Print("[ERROR] ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	savedEntries, err := getSavedEntries()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]Entry, 0)
	for _, entry := range savedEntries {
		if strings.Contains(entry.Url, query) {
			items = append(items, entry)
		}
	}

	data := struct {
		RelayName      string
		Count          uint64
		FilteredCount  uint64
		Entries        []Entry
		MainDomainName string
		Error          bool
		ErrorMessage   string
	}{
		RelayName:     s.RelayName,
		Count:         uint64(len(savedEntries)),
		FilteredCount: uint64(len(items)),
		Entries:       items,
		Error:         false,
		ErrorMessage:  "",
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	data := struct {
		RelayName        string
		RelayPubkey      string
		RelayDescription string
		RelayURL         string
		Version          string
	}{
		RelayName:        s.RelayName,
		RelayPubkey:      s.RelayPubkey,
		RelayDescription: s.RelayDescription,
		RelayURL:         s.RelayURL,
		Version:          s.Version,
	}

	respondWithJSON(w, 200, data)
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", "application/opml")
	w.Header().Add("content-disposition", "attachment; filename="+time.Now().Format(time.DateOnly)+"-rssnotes.log")
	http.ServeFile(w, r, s.LogfilePath)
}

func updateRssNotesState() {
	for {
		select {
		case followAction := <-followManagmentCh:
			updateFollowListEvent(followAction)
		case <-tickerUpdateFeeds.C:
			updateAllFeeds()
		case <-tickerDeleteOldNotes.C:
			deleteOldEvents()
		case <-quitChannel:
			tickerUpdateFeeds.Stop()
			return
		}
	}
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal JSON response %v", payload)
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}
