package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"

	"html/template"
	"rssnotes/internal/config"
	"rssnotes/internal/helpers"
	"rssnotes/internal/models"
	"rssnotes/internal/relays"
	"rssnotes/internal/yarr/yarrworker"
	"rssnotes/metrics"
	"rssnotes/server/router"
	"strconv"
	"strings"
	"time"

	"github.com/gilliek/go-opml/opml"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/skip2/go-qrcode"

	"encoding/json"
	"log"
)

var (
	recentImportedEntries []*models.GUIEntry
	importProgressCh      = make(chan models.ImportProgressStruct)
)

func (s *Server) handler() http.Handler {
	r := router.NewRouter(s.Cfg.RelayBasepath)

	r.For("/assets/*path", s.handleStatic)
	r.For("/create", (func(c *router.Context) {
		s.handleCreateFeed(c, &s.Cfg.RandomSecret)
	}))
	r.For("/import", s.handleImportOpml)
	r.For("/search", s.handleSearch)
	r.For("/progress", s.handleImportProgress)
	r.For("/detail", s.handleImportDetail)
	r.For("/export", s.handleExportOpml)
	r.For("/delete", handleDeleteFeed)
	r.For("/metrics", func(c *router.Context) {
		promhttp.Handler().ServeHTTP(c.Out, c.Req)
	})
	r.For("/metricsDisplay", s.handleMetricsDisplay)
	r.For("/log", s.handleLog)
	r.For("/health", s.handleHealth)
	r.For("/home", s.handleFrontpage)
	r.For("/", func(c *router.Context) {
		s.relay.ServeHTTP(c.Out, c.Req)
	})

	return r
}

func (s *Server) handleFrontpage(c *router.Context) {
	metrics.IndexRequests.Inc()
	items, err := relays.GetSavedEntries()
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}

	//https://appliedgo.net/spotlight/functions-in-templates-funcmap/
	funcs := template.FuncMap{
		"shortURL": func(urlLink string) string {
			u, err := url.Parse(urlLink)
			if err != nil {
				log.Printf("[ERROR] shortURL: %s", err.Error())
				return urlLink
			}
			return strings.TrimPrefix(u.Host, "www.")
		},
	}

	npub, _ := nip19.EncodePublicKey(s.Cfg.RelayPubkey)
	tmpl := template.Must(template.New("index.html").Funcs(funcs).ParseFiles(fmt.Sprintf("%s/index.html", s.Cfg.TemplatePath)))

	data := struct {
		RelayName           string
		RelayPubkey         string
		RelayNPubkey        string
		RelayDescription    string
		RelayURL            string
		Count               int
		Entries             []models.GUIEntry
		KindTextNoteCreated string
		KindTextNoteDeleted string
		QueryEventsRequests string
		NotesBlasted        string
		Version             string
	}{
		RelayName:           s.Cfg.RelayName,
		RelayPubkey:         s.Cfg.RelayPubkey,
		RelayNPubkey:        npub,
		RelayDescription:    s.Cfg.RelayDescription,
		RelayURL:            fmt.Sprintf("%s%s", s.GetAddr().Host, s.GetAddr().Path),
		Count:               len(items),
		Entries:             items,
		KindTextNoteCreated: s.getPrometheusMetric(metrics.KindTextNoteCreated.Desc()),
		KindTextNoteDeleted: s.getPrometheusMetric(metrics.KindTextNoteDeleted.Desc()),
		QueryEventsRequests: s.getPrometheusMetric(metrics.QueryEventsRequests.Desc()),
		NotesBlasted:        s.getPrometheusMetric(metrics.NotesBlasted.Desc()),
		Version:             config.Version,
	}

	if err := tmpl.Execute(c.Out, data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleStatic(c *router.Context) {
	// don't serve templates
	dir, name := filepath.Split(c.Vars["path"])
	if dir == "" && strings.HasSuffix(name, ".html") {
		c.Out.WriteHeader(http.StatusNotFound)
		return
	}
	http.StripPrefix(s.Cfg.RelayBasepath+"/assets/", http.FileServer(http.Dir(s.Cfg.StaticPath))).ServeHTTP(c.Out, c.Req)
}

func (s *Server) handleMetricsDisplay(c *router.Context) {
	items, err := relays.GetSavedEntries()
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}
	data := struct {
		Count               int
		KindTextNoteCreated string
		KindTextNoteDeleted string
		QueryEventsRequests string
		NotesBlasted        string
	}{
		Count:               len(items),
		KindTextNoteCreated: s.getPrometheusMetric(metrics.KindTextNoteCreated.Desc()),
		KindTextNoteDeleted: s.getPrometheusMetric(metrics.KindTextNoteDeleted.Desc()),
		QueryEventsRequests: s.getPrometheusMetric(metrics.QueryEventsRequests.Desc()),
		NotesBlasted:        s.getPrometheusMetric(metrics.NotesBlasted.Desc()),
	}

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/index.html", s.Cfg.TemplatePath)))
	if err := tmpl.ExecuteTemplate(c.Out, "metrics-display-fragment", data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleCreateFeed(c *router.Context, secret *string) {
	metrics.CreateRequests.Inc()
	entry := s.createFeed(c.Req, secret)

	followAction := models.FollowManagment{
		Action: models.Sync,
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
		RelayName:    s.Cfg.RelayName,
		PubKey:       entry.BookmarkEntity.PubKey,
		NPubKey:      entry.NPubKey,
		Url:          entry.BookmarkEntity.URL,
		ImageUrl:     entry.BookmarkEntity.ImageURL,
		ErrorCode:    entry.ErrorCode,
		Error:        entry.Error,
		ErrorMessage: entry.ErrorMessage,
	}

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/created.html", s.Cfg.TemplatePath)))
	err := tmpl.Execute(c.Out, data)
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) createFeed(r *http.Request, secret *string) *models.GUIEntry {
	urlParam := r.URL.Query().Get("url")

	guientry := models.GUIEntry{
		Error: false,
	}

	discFeed, err := yarrworker.DiscoverRssFeed(urlParam)
	if err != nil || discFeed.FeedLink == "" {
		guientry.ErrorCode = http.StatusBadRequest
		guientry.Error = true
		guientry.ErrorMessage = "Could not find a feed URL in there..."
		return &guientry
	}
	feedUrl := discFeed.FeedLink

	sk := relays.GetPrivateKeyFromFeedUrl(feedUrl, *secret)
	publicKey, err := nostr.GetPublicKey(sk)
	if err != nil {
		guientry.ErrorCode = http.StatusInternalServerError
		guientry.Error = true
		guientry.ErrorMessage = "Bad private key: " + err.Error()
		log.Printf("[ERROR] bad private key from feed: %s", err)
		return &guientry
	}

	publicKey = strings.TrimSpace(publicKey)

	if feedExists, err := relays.FeedExists(publicKey, sk, feedUrl); err != nil || feedExists {
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

	parsedFeed, err := relays.ParseFeedForUrl(feedUrl)
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
	guientry.BookmarkEntity.ImageURL = s.Cfg.DefaultProfilePicUrl

	faviconUrl, err := yarrworker.FindFaviconURL(parsedFeed.Link, feedUrl)
	if err != nil {
		log.Print("[ERROR] FindFavicon", err)
	} else if faviconUrl != "" {
		guientry.BookmarkEntity.ImageURL = faviconUrl
	}

	if err := relays.CreateMetadataNote(publicKey, sk, parsedFeed, guientry.BookmarkEntity.ImageURL); err != nil {
		log.Printf("[ERROR] creating metadata note %s", err)
	}

	// if _, metadataEvent, _ := getLocalMetadataEvent(publicKey); metadataEvent.ID != "" {
	// 	publishNostrEventCh <- metadataEvent
	// }

	lastPostTime, allPostTimes := relays.InitFeed(publicKey, sk, feedUrl, parsedFeed)

	if err := relays.AddEntityToBookmarkEvent([]models.Entity{
		{PubKey: publicKey,
			PrivateKey:      sk,
			URL:             feedUrl,
			ImageURL:        guientry.BookmarkEntity.ImageURL,
			LastPostTime:    lastPostTime,
			LastCheckedTime: time.Now().Unix(),
			AvgPostTime:     relays.CalcAvgPostTime(allPostTimes)}}); err != nil {
		log.Printf("[ERROR] feed entity %s not added to bookmark", feedUrl)
	}

	if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", guientry.NPubKey), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.Cfg.QRCodePath, guientry.NPubKey)); err != nil {
		log.Print("[ERROR]", err)
	}

	return &guientry
}

func handleDeleteFeed(c *router.Context) {
	metrics.DeleteRequests.Inc()
	feedPubkey := c.Req.URL.Query().Get("pubkey")

	followAction := models.FollowManagment{
		Action:       models.Delete,
		FollowEntity: models.Entity{PubKey: feedPubkey},
	}
	followManagmentCh <- followAction

	if err := relays.DeleteEntityInBookmarkEvent(feedPubkey); err != nil {
		log.Printf("[ERROR] could not delete feed '%q'...Error: %s ", feedPubkey, err)
	}

	tmpl := template.New("t")
	tmpl.Execute(c.Out, nil)
}

func (s *Server) handleImportOpml(c *router.Context) {
	metrics.ImportRequests.Inc()
	outputFileStatus := func(errMsg string) {
		htmlStr := fmt.Sprintf("<div id='progress' name='progress-bar' class='progress-bar' style='--width: 100' data-label='%s...'></div>", errMsg)
		tmplProg, _ := template.New("t").Parse(htmlStr)
		tmplProg.Execute(c.Out, nil)
	}

	if err := c.Req.ParseMultipartForm(10 << 20); err != nil {
		errMsg := fmt.Sprintf("[ERROR] OPML parse form %s", err)
		log.Print(errMsg)
		outputFileStatus("[ERROR] OPML parse form")
		return
	}

	file, _, err := c.Req.FormFile("opml-file")
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
		recentImportedEntries = s.importFeeds(doc.Body.Outlines, &s.Cfg.RandomSecret)
	}()

	log.Print("[DEBUG] opml import started.")
	outputFileStatus("OPML import starting")
}

func (s *Server) importFeeds(opmlUrls []opml.Outline, secret *string) []*models.GUIEntry {
	importedEntries := make([]*models.GUIEntry, 0)
	bookmarkEntities := make([]models.Entity, 0)

	for urlIndex, urlParam := range opmlUrls {
		if !helpers.IsValidHttpUrl(urlParam.XMLURL) {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Invalid URL provided (must be in absolute format and with https or https scheme)...",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- models.ImportProgressStruct{EntryIndex: urlIndex, TotalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] invalid feed url '%q' skipping...", urlParam)
			continue
		}

		discFeed, err := yarrworker.DiscoverRssFeed(urlParam.XMLURL)
		if err != nil || discFeed.FeedLink == "" {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Could not find a feed URL in there...",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- models.ImportProgressStruct{EntryIndex: urlIndex, TotalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] Could not find a feed URL in %s", urlParam.XMLURL)
			continue
		}
		feedUrl := discFeed.FeedLink

		sk := relays.GetPrivateKeyFromFeedUrl(feedUrl, *secret)
		publicKey, err := nostr.GetPublicKey(sk)
		if err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Bad private key",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- models.ImportProgressStruct{EntryIndex: urlIndex, TotalEntries: len(opmlUrls)}
			log.Printf("[ERROR] feed %s bad private key: %s", feedUrl, err)
			continue
		}

		publicKey = strings.TrimSpace(publicKey)

		feedExists, err := relays.FeedExists(publicKey, sk, feedUrl)
		if feedExists {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Feed already exists",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- models.ImportProgressStruct{EntryIndex: urlIndex, TotalEntries: len(opmlUrls)}
			log.Printf("[DEBUG] feedUrl %s with pubkey %s already exists", feedUrl, publicKey)
			continue
		} else if err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Could not determine if feed exists",
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- models.ImportProgressStruct{EntryIndex: urlIndex, TotalEntries: len(opmlUrls)}
			log.Printf("[ERROR] could not determine if feedUrl %s with pubkey %s exists", feedUrl, publicKey)
			continue
		}

		parsedFeed, err := relays.ParseFeedForUrl(feedUrl)
		if err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{URL: urlParam.XMLURL},
				ErrorMessage:   "Can not parse feed: " + err.Error(),
				Error:          true,
				ErrorCode:      http.StatusBadRequest,
			})
			importProgressCh <- models.ImportProgressStruct{EntryIndex: urlIndex, TotalEntries: len(opmlUrls)}
			log.Printf("[ERROR] can not parse feed %s", err)
			continue
		}

		npub, _ := nip19.EncodePublicKey(publicKey)
		guiEntry := models.GUIEntry{
			BookmarkEntity: models.Entity{URL: urlParam.XMLURL, PubKey: publicKey},
			NPubKey:        npub,
			ErrorMessage:   "",
			Error:          false,
			ErrorCode:      0,
		}

		if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", guiEntry.NPubKey), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.Cfg.QRCodePath, guiEntry.NPubKey)); err != nil {
			log.Print("[ERROR] ", err)
		}

		localImageURL := s.Cfg.DefaultProfilePicUrl
		faviconUrl, err := yarrworker.FindFaviconURL(parsedFeed.Link, feedUrl)
		if err != nil {
			log.Print("[ERROR] FindFavicon", err)
		} else if faviconUrl != "" {
			localImageURL = faviconUrl
		}

		if err := relays.CreateMetadataNote(publicKey, sk, parsedFeed, localImageURL); err != nil {
			log.Printf("[ERROR] creating metadata note %s", err)
		}

		lastPostTime, allPostTimes := relays.InitFeed(publicKey, sk, feedUrl, parsedFeed)
		bookmarkEntities = append(bookmarkEntities, models.Entity{
			PubKey:          publicKey,
			PrivateKey:      sk,
			URL:             feedUrl,
			ImageURL:        localImageURL,
			LastPostTime:    lastPostTime,
			LastCheckedTime: time.Now().Unix(),
			AvgPostTime:     relays.CalcAvgPostTime(allPostTimes),
		})

		importedEntries = append(importedEntries, &guiEntry)
		importProgressCh <- models.ImportProgressStruct{EntryIndex: urlIndex, TotalEntries: len(opmlUrls)}
	}

	if err := relays.AddEntityToBookmarkEvent(bookmarkEntities); err != nil {
		log.Printf("[ERROR] adding feed entities: %s", err)
	}

	//update kind 3 event
	followAction := models.FollowManagment{
		Action: models.Sync,
	}
	followManagmentCh <- followAction

	return importedEntries
}

func (s *Server) handleImportProgress(c *router.Context) {
	importedURL := <-importProgressCh
	progressPct := ((float32(importedURL.EntryIndex) + 1.0) / float32(importedURL.TotalEntries)) * 100.0

	if importedURL.EntryIndex+1 < importedURL.TotalEntries {
		htmlStr := fmt.Sprintf("<div class='navbar-item' id='status-area' hx-get='./progress' hx-target='this' hx-swap='outerHTML' hx-trigger='every 600ms'>Processing...%d of %d<div class='navbar-item'><div id='progress' name='progress-bar' class='progress-bar' style='--width: %f' data-label=''></div></div></div>", importedURL.EntryIndex+1, importedURL.TotalEntries, progressPct)
		tmpl, _ := template.New("t").Parse(htmlStr)
		tmpl.Execute(c.Out, nil)
	} else {
		htmlStr := fmt.Sprintf("<div class='navbar-item' id='status-area' hx-get='./progress' hx-target='this' hx-swap='outerHTML' hx-trigger='change from:#opml-import-form' hx-sync='#opml-file: queue first'><a href='./'>Refresh</a>..or..<a href='./detail'>Details</a> <div class='navbar-item'><div id='progress' name='progress-bar' class='progress-bar' style='--width: %f' data-label='Import Complete...'></div></div></div>", progressPct)
		tmpl, _ := template.New("t").Parse(htmlStr)
		tmpl.Execute(c.Out, nil)
	}
}

func (s *Server) handleImportDetail(c *router.Context) {
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/imported.html", s.Cfg.TemplatePath)))

	numBadFeeds := 0
	for _, feed := range recentImportedEntries {
		if feed.Error {
			numBadFeeds++
		}
	}

	results := struct {
		RelayName    string
		Feeds        []*models.GUIEntry
		GoodFeeds    int
		BadFeeds     int
		Error        bool
		ErrorMessage string
		ErrorCode    int
	}{
		RelayName:    s.Cfg.RelayName,
		Feeds:        recentImportedEntries,
		GoodFeeds:    len(recentImportedEntries) - numBadFeeds,
		BadFeeds:     numBadFeeds,
		Error:        false,
		ErrorMessage: "OPML File Processed",
		ErrorCode:    0,
	}

	if err := tmpl.Execute(c.Out, results); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleExportOpml(c *router.Context) {
	var rssOMPL = &opml.OPML{
		Version: "1.0",
		Head: opml.Head{
			Title:       "rssnotes Feeds",
			DateCreated: time.Now().Format(time.RFC3339),
			OwnerName:   "rssnotes",
		},
	}

	data, _ := relays.GetSavedEntities()

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

	c.Out.Header().Add("content-type", "application/opml")
	c.Out.Header().Add("content-disposition", "attachment; filename="+time.Now().Format(time.DateOnly)+"-rssnotes.opml")
	outp, err := rssOMPL.XML()
	if err != nil {
		log.Print("[ERROR] exporting opml file")
		http.Redirect(c.Out, c.Req, c.Req.Referer(), http.StatusSeeOther)
		return
	}

	fmt.Fprintf(c.Out, "%s", outp)
}

func (s *Server) handleSearch(c *router.Context) {
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/search.html", s.Cfg.TemplatePath)))
	metrics.SearchRequests.Inc()
	query := c.Req.URL.Query().Get("query")
	if query == "" || len(query) <= 4 {

		errorData := struct {
			RelayName     string
			Count         uint64
			FilteredCount uint64
			Entries       []models.GUIEntry
			Error         bool
			ErrorMessage  string
		}{
			RelayName:     s.Cfg.RelayName,
			Count:         0,
			FilteredCount: 0,
			Entries:       nil,
			Error:         true,
			ErrorMessage:  "Please enter more than 5 characters to search!",
		}

		if err := tmpl.Execute(c.Out, errorData); err != nil {
			log.Print("[ERROR] ", err)
			http.Error(c.Out, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	savedEntries, err := relays.GetSavedEntries()
	if err != nil {
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]models.GUIEntry, 0)
	for _, entry := range savedEntries {
		if strings.Contains(entry.BookmarkEntity.URL, query) {
			items = append(items, entry)
		}
	}

	data := struct {
		RelayName     string
		Count         uint64
		FilteredCount uint64
		Entries       []models.GUIEntry
		Error         bool
		ErrorMessage  string
	}{
		RelayName:     s.Cfg.RelayName,
		Count:         uint64(len(savedEntries)),
		FilteredCount: uint64(len(items)),
		Entries:       items,
		Error:         false,
		ErrorMessage:  "",
	}

	if err := tmpl.Execute(c.Out, data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleHealth(c *router.Context) {
	data := struct {
		RelayName        string
		RelayPubkey      string
		RelayDescription string
		RelayURL         string
		Version          string
		GitHash          string
		ReleaseDate      string
	}{
		RelayName:        s.Cfg.RelayName,
		RelayPubkey:      s.Cfg.RelayPubkey,
		RelayDescription: s.Cfg.RelayDescription,
		RelayURL:         s.Cfg.RelayURL,
		Version:          config.Version,
		GitHash:          config.GitHash,
		ReleaseDate:      config.ReleaseDate,
	}

	respondWithJSON(c, 200, data)
}

func (s *Server) handleLog(c *router.Context) {
	c.Out.Header().Add("content-type", "application/opml")
	c.Out.Header().Add("content-disposition", "attachment; filename="+time.Now().Format(time.DateOnly)+"-rssnotes.log")
	http.ServeFile(c.Out, c.Req, s.Cfg.LogfilePath)
}

func respondWithJSON(c *router.Context, code int, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal JSON response %v", payload)
		c.Out.WriteHeader(500)
		return
	}
	c.Out.Header().Add("Content-Type", "application/json")
	c.Out.Header().Add("Access-Control-Allow-Origin", "*")
	c.Out.WriteHeader(code)
	c.Out.Write(data)
}

func (s *Server) getPrometheusMetric(promParam *prometheus.Desc) string {

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 {
				return errors.New("stopped after 2 redirects")
			}
			return nil
		},
		Timeout: 3 * time.Second,
	}

	url := fmt.Sprintf("http://localhost:%s%s/metrics", s.Cfg.Port, s.Cfg.RelayBasepath)

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
			return helpers.NearestThousandFormat(float64(countInt64))
		}
	}
	return "?"
}
