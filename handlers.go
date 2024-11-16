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

var (
	recentImportedEntries []*Entry
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
	outputFileStatus := func(errMsg string) {
		htmlStr := fmt.Sprintf("<div id='progress' name='progress-bar' class='progress-bar' style='--width: 100' data-label='%s...'></div>", errMsg)
		tmplProg, _ := template.New("t").Parse(htmlStr)
		tmplProg.Execute(w, nil)
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		errMsg := fmt.Sprintf("[ERROR] OPML parse form %s", err)
		log.Print(errMsg)
		outputFileStatus("[ERROR] OPML parse form")
		return
	}

	file, _, err := r.FormFile("opml-file")
	if err != nil {
		errMsg := fmt.Sprintf("[ERROR] form OPML file: %s", err)
		log.Print(errMsg)
		outputFileStatus("[ERROR] form OPML file")
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		errMsg := fmt.Sprintf("[ERROR] reading OPML file: %s", err)
		log.Print(errMsg)
		outputFileStatus("[ERROR] reading OPML file")
		return
	}

	doc, err := opml.NewOPML(fileBytes)
	if err != nil {
		errMsg := fmt.Sprintf("[ERROR] OPML bad file format %s", err)
		log.Print(errMsg)
		outputFileStatus("[ERROR] OPML bad file format")
		return
	}

	go func() {
		recentImportedEntries = importFeeds(doc.Body.Outlines, &s.RandomSecret)
	}()

	log.Print("[DEBUG] opml import started.")
	outputFileStatus("OPML import starting")
}

func importFeeds(opmlUrls []opml.Outline, secret *string) []*Entry {
	importedEntries := make([]*Entry, 0)
	bookmarkEntities := make([]Entity, 0)

	for urlIndex, urlParam := range opmlUrls {
		if !IsValidHttpUrl(urlParam.XMLURL) {
			importedEntries = append(importedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Invalid URL provided (must be in absolute format and with https or https scheme)...",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] invalid feed url '%q' skipping...", urlParam)
			continue
		}

		feedUrl := getFeedUrl(urlParam.XMLURL)
		if feedUrl == "" {
			importedEntries = append(importedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Could not find a feed URL in there...",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] Could not find a feed URL in %s", feedUrl)
			continue
		}

		sk := getPrivateKeyFromFeedUrl(feedUrl, *secret)
		publicKey, err := nostr.GetPublicKey(sk)
		if err != nil {
			importedEntries = append(importedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Bad private key",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[ERROR] feed %s bad private key: %s", feedUrl, err)
			continue
		}

		publicKey = strings.TrimSpace(publicKey)

		feedExists, err := feedExists(publicKey, sk, feedUrl)
		if feedExists {
			importedEntries = append(importedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Feed already exists",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] feedUrl %s with pubkey %s already exists", feedUrl, publicKey)
			continue
		} else if err != nil {
			importedEntries = append(importedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Could not determine if feed exists",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[ERROR] could not determine if feedUrl %s with pubkey %s exists", feedUrl, publicKey)
			continue
		}

		parsedFeed, err := parseFeedForUrl(feedUrl)
		if err != nil {
			importedEntries = append(importedEntries, &Entry{
				Url:          urlParam.XMLURL,
				ErrorMessage: "Can not parse feed: " + err.Error(),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[ERROR] can not parse feed %s", err)
			continue
		}

		npub, _ := nip19.EncodePublicKey(publicKey)
		guiEntry := Entry{
			Url:          feedUrl,
			PubKey:       publicKey,
			NPubKey:      npub,
			ErrorMessage: "",
			Error:        false,
			ErrorCode:    0,
		}
		importedEntries = append(importedEntries, &guiEntry)
		importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}

		if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", guiEntry.NPubKey), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.QRCodePath, guiEntry.NPubKey)); err != nil {
			log.Print("[ERROR] ", err)
		}

		if err := createMetadataNote(publicKey, sk, parsedFeed, s.DefaultProfilePicUrl); err != nil {
			log.Printf("[ERROR] creating metadata note %s", err)
		}

		latestCreatedAt := initFeed(publicKey, sk, feedUrl, parsedFeed)
		bookmarkEntities = append(bookmarkEntities, Entity{PublicKey: publicKey, PrivateKey: sk, URL: feedUrl, LastUpdate: latestCreatedAt})
	}

	if err := addEntityToBookmarkEvent(bookmarkEntities); err != nil {
		log.Printf("[ERROR] adding feed entities: %s", err)
	}

	//update kind 3 event
	followAction := FollowManagment{
		Action: Sync,
	}
	followManagmentCh <- followAction

	return importedEntries
}

func handleImportProgress(w http.ResponseWriter, r *http.Request) {
	importedURL := <-importProgressCh
	progressPct := ((float32(importedURL.entryIndex) + 1.0) / float32(importedURL.totalEntries)) * 100.0

	if importedURL.entryIndex+1 < importedURL.totalEntries {
		htmlStr := fmt.Sprintf("<div class='navbar-item' id='status-area' hx-get='/progress' hx-target='this' hx-swap='outerHTML' hx-trigger='every 600ms'>Processing...%d of %d<div class='navbar-item'><div id='progress' name='progress-bar' class='progress-bar' style='--width: %f' data-label=''></div></div></div>", importedURL.entryIndex+1, importedURL.totalEntries, progressPct)
		tmpl, _ := template.New("t").Parse(htmlStr)
		tmpl.Execute(w, nil)
	} else {
		htmlStr := fmt.Sprintf("<div class='navbar-item' id='status-area' hx-get='/progress' hx-target='this' hx-swap='outerHTML' hx-trigger='change from:#opml-import-form' hx-sync='#opml-file: queue first'><a href='/'>Refresh</a>..or..<a href='/detail'>Details</a> <div class='navbar-item'><div id='progress' name='progress-bar' class='progress-bar' style='--width: %f' data-label='Import Complete...'></div></div></div>", progressPct)
		tmpl, _ := template.New("t").Parse(htmlStr)
		tmpl.Execute(w, nil)
	}
}

func handleImportDetail(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/imported.html", s.TemplatePath)))

	numBadFeeds := 0
	for _, feed := range recentImportedEntries {
		if feed.Error {
			numBadFeeds++
		}
	}

	results := struct {
		RelayName    string
		Feeds        []*Entry
		GoodFeeds    int
		BadFeeds     int
		Error        bool
		ErrorMessage string
		ErrorCode    int
	}{
		RelayName:    s.RelayName,
		Feeds:        recentImportedEntries,
		GoodFeeds:    len(recentImportedEntries) - numBadFeeds,
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
