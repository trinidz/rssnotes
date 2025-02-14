package main

import (
	"fmt"
	"io"
	"net/http"
	"rssnotes/metrics"

	//"rssnotes/rssworker"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/gilliek/go-opml/opml"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/skip2/go-qrcode"

	"encoding/json"
	"log"
)

var (
	recentImportedEntries []*GUIEntry
)

func handleFrontpage(w http.ResponseWriter, _ *http.Request) {
	metrics.IndexRequests.Inc()
	items, err := getSavedEntries()
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	npub, _ := nip19.EncodePublicKey(s.RelayPubkey)
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/index.html", s.TemplatePath)))

	data := struct {
		RelayName           string
		RelayPubkey         string
		RelayNPubkey        string
		RelayDescription    string
		RelayURL            string
		Count               int
		Entries             []GUIEntry
		KindTextNoteCreated string
		KindTextNoteDeleted string
		QueryEventsRequests string
		NotesBlasted        string
	}{
		RelayName:           s.RelayName,
		RelayPubkey:         s.RelayPubkey,
		RelayNPubkey:        npub,
		RelayDescription:    s.RelayDescription,
		RelayURL:            s.RelayURL,
		Count:               len(items),
		Entries:             items,
		KindTextNoteCreated: getPrometheusMetric(metrics.KindTextNoteCreated.Desc()),
		KindTextNoteDeleted: getPrometheusMetric(metrics.KindTextNoteDeleted.Desc()),
		QueryEventsRequests: getPrometheusMetric(metrics.QueryEventsRequests.Desc()),
		NotesBlasted:        getPrometheusMetric(metrics.NotesBlasted.Desc()),
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleMetricsDisplay(w http.ResponseWriter, _ *http.Request) {
	items, err := getSavedEntries()
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	data := struct {
		Count               int
		KindTextNoteCreated string
		KindTextNoteDeleted string
		QueryEventsRequests string
		NotesBlasted        string
	}{
		Count:               len(items),
		KindTextNoteCreated: getPrometheusMetric(metrics.KindTextNoteCreated.Desc()),
		KindTextNoteDeleted: getPrometheusMetric(metrics.KindTextNoteDeleted.Desc()),
		QueryEventsRequests: getPrometheusMetric(metrics.QueryEventsRequests.Desc()),
		NotesBlasted:        getPrometheusMetric(metrics.NotesBlasted.Desc()),
	}

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/index.html", s.TemplatePath)))
	if err := tmpl.ExecuteTemplate(w, "metrics-display-fragment", data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleCreateFeed(w http.ResponseWriter, r *http.Request, secret *string) {
	metrics.CreateRequests.Inc()
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
		ImageUrl     string
		ErrorCode    int
		Error        bool
		ErrorMessage string
	}{
		RelayName:    s.RelayName,
		PubKey:       entry.BookmarkEntity.PubKey,
		NPubKey:      entry.NPubKey,
		Url:          entry.BookmarkEntity.URL,
		ImageUrl:     entry.BookmarkEntity.ImageURL,
		ErrorCode:    entry.ErrorCode,
		Error:        entry.Error,
		ErrorMessage: entry.ErrorMessage,
	}

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/created.html", s.TemplatePath)))
	err := tmpl.Execute(w, data)
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func createFeed(r *http.Request, secret *string) *GUIEntry {
	urlParam := r.URL.Query().Get("url")

	guientry := GUIEntry{
		Error: false,
	}

	if !IsValidHttpUrl(urlParam) {
		log.Printf("[DEBUG] tried to create feed from invalid feed url '%q' skipping...", urlParam)
		guientry.ErrorCode = http.StatusBadRequest
		guientry.Error = true
		guientry.ErrorMessage = "Invalid URL provided (must be in absolute format and with https or https scheme)..."
		return &guientry
	}

	feedUrl := getFeedUrl(urlParam)
	if feedUrl == "" {
		guientry.ErrorCode = http.StatusBadRequest
		guientry.Error = true
		guientry.ErrorMessage = "Could not find a feed URL in there..."
		return &guientry
	}

	sk := getPrivateKeyFromFeedUrl(feedUrl, *secret)
	publicKey, err := nostr.GetPublicKey(sk)
	if err != nil {
		guientry.ErrorCode = http.StatusInternalServerError
		guientry.Error = true
		guientry.ErrorMessage = "Bad private key: " + err.Error()
		log.Printf("[ERROR] bad private key from feed: %s", err)
		return &guientry
	}

	publicKey = strings.TrimSpace(publicKey)

	if feedExists, err := feedExists(publicKey, sk, feedUrl); err != nil || feedExists {
		if feedExists {
			log.Printf("[DEBUG] feedUrl %s with pubkey %s already exists", feedUrl, publicKey)
			guientry.ErrorMessage = fmt.Sprintf("Feed %s already exists", feedUrl)
		} else {
			log.Printf("[ERROR] could not determine if feedUrl %s with pubkey %s exists", feedUrl, publicKey)
			guientry.ErrorMessage = fmt.Sprintf("Could not determine if feed %s exists", feedUrl)
		}
		guientry.ErrorCode = http.StatusInternalServerError
		guientry.Error = true
		return &guientry
	}

	parsedFeed, err := parseFeedForUrl(feedUrl)
	if err != nil || parsedFeed == nil {
		guientry.ErrorCode = http.StatusBadRequest
		guientry.Error = true
		guientry.ErrorMessage = "Can not parse feed: " + err.Error()
		log.Printf("[ERROR] can not parse feed %s", err)
		return &guientry
	}

	guientry.BookmarkEntity.URL = feedUrl
	guientry.BookmarkEntity.PubKey = publicKey
	guientry.NPubKey, _ = nip19.EncodePublicKey(publicKey)
	guientry.BookmarkEntity.ImageURL = s.DefaultProfilePicUrl

	faviconUrl, err := worker.FindFaviconURL(parsedFeed.Link, feedUrl)
	if err != nil {
		log.Print("[ERROR] FindFavicon", err)
	} else if faviconUrl != "" {
		guientry.BookmarkEntity.ImageURL = faviconUrl
	}

	if err := createMetadataNote(publicKey, sk, parsedFeed, guientry.BookmarkEntity.ImageURL); err != nil {
		log.Printf("[ERROR] creating metadata note %s", err)
	}

	// if _, metadataEvent, _ := getLocalMetadataEvent(publicKey); metadataEvent.ID != "" {
	// 	publishNostrEventCh <- metadataEvent
	// }

	lastPostTime, allPostTimes := initFeed(publicKey, sk, feedUrl, parsedFeed)

	if err := addEntityToBookmarkEvent([]Entity{
		{PubKey: publicKey,
			PrivateKey:      sk,
			URL:             feedUrl,
			ImageURL:        guientry.BookmarkEntity.ImageURL,
			LastPostTime:    lastPostTime,
			LastCheckedTime: time.Now().Unix(),
			AvgPostTime:     CalcAvgPostTime(allPostTimes)}}); err != nil {
		log.Printf("[ERROR] feed entity %s not added to bookmark", feedUrl)
	}

	if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", guientry.NPubKey), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.QRCodePath, guientry.NPubKey)); err != nil {
		log.Print("[ERROR]", err)
	}

	return &guientry
}

func handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	metrics.DeleteRequests.Inc()
	feedPubkey := r.URL.Query().Get("pubkey")

	followAction := FollowManagment{
		Action:       Delete,
		FollowEntity: Entity{PubKey: feedPubkey},
	}
	followManagmentCh <- followAction

	if err := deleteEntityInBookmarkEvent(feedPubkey); err != nil {
		log.Printf("[ERROR] could not delete feed '%q'...Error: %s ", feedPubkey, err)
	}

	tmpl := template.New("t")
	tmpl.Execute(w, nil)
}

func handleImportOpml(w http.ResponseWriter, r *http.Request) {
	metrics.ImportRequests.Inc()
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

func importFeeds(opmlUrls []opml.Outline, secret *string) []*GUIEntry {
	importedEntries := make([]*GUIEntry, 0)
	bookmarkEntities := make([]Entity, 0)

	for urlIndex, urlParam := range opmlUrls {
		if !IsValidHttpUrl(urlParam.XMLURL) {
			importedEntries = append(importedEntries, &GUIEntry{
				BookmarkEntity: Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Invalid URL provided (must be in absolute format and with https or https scheme)...",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] invalid feed url '%q' skipping...", urlParam)
			continue
		}

		feedUrl := getFeedUrl(urlParam.XMLURL)
		if feedUrl == "" {
			importedEntries = append(importedEntries, &GUIEntry{
				BookmarkEntity: Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Could not find a feed URL in there...",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] Could not find a feed URL in %s", feedUrl)
			continue
		}

		sk := getPrivateKeyFromFeedUrl(feedUrl, *secret)
		publicKey, err := nostr.GetPublicKey(sk)
		if err != nil {
			importedEntries = append(importedEntries, &GUIEntry{
				BookmarkEntity: Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Bad private key",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[ERROR] feed %s bad private key: %s", feedUrl, err)
			continue
		}

		publicKey = strings.TrimSpace(publicKey)

		feedExists, err := feedExists(publicKey, sk, feedUrl)
		if feedExists {
			importedEntries = append(importedEntries, &GUIEntry{
				BookmarkEntity: Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Feed already exists",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] feedUrl %s with pubkey %s already exists", feedUrl, publicKey)
			continue
		} else if err != nil {
			importedEntries = append(importedEntries, &GUIEntry{
				BookmarkEntity: Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Could not determine if feed exists",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[ERROR] could not determine if feedUrl %s with pubkey %s exists", feedUrl, publicKey)
			continue
		}

		parsedFeed, err := parseFeedForUrl(feedUrl)
		if err != nil {
			importedEntries = append(importedEntries, &GUIEntry{
				BookmarkEntity: Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Can not parse feed: " + err.Error(),
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
			log.Printf("[ERROR] can not parse feed %s", err)
			continue
		}

		npub, _ := nip19.EncodePublicKey(publicKey)
		guiEntry := GUIEntry{
			BookmarkEntity: Entity{URL: urlParam.XMLURL, PubKey: publicKey},
			NPubKey:        npub,
			ErrorMessage:   "",
			Error:          false,
			ErrorCode:      0,
		}

		if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", guiEntry.NPubKey), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.QRCodePath, guiEntry.NPubKey)); err != nil {
			log.Print("[ERROR] ", err)
		}

		localImageURL := s.DefaultProfilePicUrl
		faviconUrl, err := worker.FindFaviconURL(parsedFeed.Link, feedUrl)
		if err != nil {
			log.Print("[ERROR] FindFavicon", err)
		} else if faviconUrl != "" {
			localImageURL = faviconUrl
		}

		if err := createMetadataNote(publicKey, sk, parsedFeed, localImageURL); err != nil {
			log.Printf("[ERROR] creating metadata note %s", err)
		}

		lastPostTime, allPostTimes := initFeed(publicKey, sk, feedUrl, parsedFeed)
		bookmarkEntities = append(bookmarkEntities, Entity{
			PubKey:          publicKey,
			PrivateKey:      sk,
			URL:             feedUrl,
			ImageURL:        localImageURL,
			LastPostTime:    lastPostTime,
			LastCheckedTime: time.Now().Unix(),
			AvgPostTime:     CalcAvgPostTime(allPostTimes),
		})

		importedEntries = append(importedEntries, &guiEntry)
		importProgressCh <- ImportProgressStruct{entryIndex: urlIndex, totalEntries: len(opmlUrls)}
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
		Feeds        []*GUIEntry
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
			Title:       "rssnotes Feeds",
			DateCreated: time.Now().Format(time.RFC3339),
			OwnerName:   "rssnotes",
		},
	}

	data, _ := getSavedEntities()

	for _, feed := range data {
		rssOMPL.Body.Outlines = append(rssOMPL.Body.Outlines, opml.Outline{
			Type: "rss",
			Text: feed.PubKey,
			//Title:   feed.PrivateKey,
			XMLURL:  feed.URL,
			HTMLURL: feed.URL,
			//Created: strconv.FormatInt(feed.LastUpdate, 10),
		})
	}

	w.Header().Add("content-type", "application/opml")
	w.Header().Add("content-disposition", "attachment; filename="+time.Now().Format(time.DateOnly)+"-rssnotes.opml")
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
	metrics.SearchRequests.Inc()
	query := r.URL.Query().Get("query")
	if query == "" || len(query) <= 4 {

		errorData := struct {
			RelayName     string
			Count         uint64
			FilteredCount uint64
			Entries       []GUIEntry
			Error         bool
			ErrorMessage  string
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

	items := make([]GUIEntry, 0)
	for _, entry := range savedEntries {
		if strings.Contains(entry.BookmarkEntity.URL, query) {
			items = append(items, entry)
		}
	}

	data := struct {
		RelayName     string
		Count         uint64
		FilteredCount uint64
		Entries       []GUIEntry
		Error         bool
		ErrorMessage  string
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
		case nostrEvent := <-publishNostrEventCh:
			go func() {
				blastEvent(&nostrEvent)
			}()
		case <-tickerUpdateFeeds.C:
			checkAllFeeds()
		case <-tickerDeleteOldNotes.C:
			deleteOldKindTextNoteEvents()
		case <-quitChannel:
			tickerUpdateFeeds.Stop()
			tickerDeleteOldNotes.Stop()
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
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	w.Write(data)
}

func getPrometheusMetric(promParam *prometheus.Desc) string {
	url := fmt.Sprintf("http://localhost:%s/metrics", s.Port)

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return "?"
	} else if resp.StatusCode >= 300 {
		log.Printf("[ERROR] status code: %d", resp.StatusCode)
		return "?"
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Print(err)
		return "?"
	}

	respString := string(respBytes)
	respLines := strings.Split(respString, "\n")
	promMetricName := strings.Split(promParam.String(), "\"")[1]

	for _, line := range respLines {
		if strings.HasPrefix(line, promMetricName) {
			//fmt.Println(line)
			count := strings.Split(line, " ")[1]
			countInt64, err := strconv.ParseInt(count, 10, 64)
			if err != nil {
				log.Print("[ERROR]", err)
				return "?"
			}
			return nearestThousandFormat(float64(countInt64))
		}
	}
	return "?"
}
