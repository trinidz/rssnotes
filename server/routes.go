package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rssnotes/internal/config"
	"rssnotes/internal/helpers"
	"rssnotes/internal/models"
	"rssnotes/internal/relays"
	"rssnotes/internal/yarr/yarrworker"
	"rssnotes/metrics"
	"rssnotes/server/router"

	"github.com/gilliek/go-opml/opml"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/skip2/go-qrcode"
)

type ImportProgressStruct struct {
	EntryIndex   int
	TotalEntries int
}

var (
	recentImportedEntries []*models.GUIEntry
	importProgressCh      = make(chan ImportProgressStruct)
)

func (s *Server) handler() http.Handler {
	r := router.NewRouter(s.Cfg.RelayBasepath)

	r.For("/assets/*path", s.handleStatic)
	r.For("/create", s.handleGeneratePubkeyBtn)
	r.For("/search", s.handleSearchBtn)
	r.For("/blastfeed", handleBlastFeed)
	r.For("/import", s.handleImportOpml)
	r.For("/progress", handleImportProgress)
	r.For("/detail", s.handleImportDetail)
	r.For("/export", s.handleExportOpml)
	r.For("/delete", handleDeleteBtn)
	r.For("/metrics", func(c *router.Context) {
		promhttp.Handler().ServeHTTP(c.Out, c.Req)
	})
	r.For("/metricsDisplay", s.handleMetricsDisplay)
	r.For("/log", s.handleLog)
	r.For("/health", s.handleHealth)
	r.For("/profile", s.handleProfileBtn)
	r.For("/profilesave", s.handleProfileSaveBtn)
	r.For("/home", s.handleFrontpage)
	r.For("/", func(c *router.Context) {
		s.relay.ServeHTTP(c.Out, c.Req)
	})

	return r
}

func (s *Server) handleFrontpage(c *router.Context) {
	metrics.IndexRequests.Inc()
	items, err := relays.GetEntries()
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}

	//https://appliedgo.net/spotlight/functions-in-templates-funcmap/
	/*
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
	*/

	npub, _ := nip19.EncodePublicKey(s.Cfg.RelayPubkey)

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/index.html", s.Cfg.TemplatePath)))
	//tmpl := template.Must(template.New("index.html").Funcs(funcs).ParseFiles(fmt.Sprintf("%s/index.html", s.Cfg.TemplatePath)))

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

func (s *Server) handleProfileBtn(c *router.Context) {

	feedPubkey := c.Req.URL.Query().Get("pubkey")

	feedMetadata, _ := relays.GetLocalMetadataEvent(feedPubkey)
	if feedMetadata.ID == "" {
		log.Print("[ERROR] metadata event not found")
		c.Out.WriteHeader(http.StatusNoContent) // 204 - No Content.
		return
	}

	nostrProfile := models.Profile{}

	if err := json.Unmarshal([]byte(feedMetadata.Content), &nostrProfile); err != nil {
		log.Print("[ERROR] unmarshal profile: ", err)
		c.Out.WriteHeader(http.StatusNoContent) // 204 - No Content.
		return
	}

	data := struct {
		Profile models.Profile
		Pubkey  string
	}{
		Profile: models.Profile{
			DisplayName: nostrProfile.DisplayName,
			Name:        nostrProfile.Name,
			Nip05:       nostrProfile.Nip05,
			Lud16:       nostrProfile.Lud16,
			Picture:     nostrProfile.Picture,
			Banner:      nostrProfile.Banner,
			Website:     nostrProfile.Website,
			About:       nostrProfile.About},
		Pubkey: feedPubkey,
	}

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/profile.html", s.Cfg.TemplatePath)))

	if err := tmpl.Execute(c.Out, data); err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleProfileSaveBtn(c *router.Context) {

	if c.Req.Method != http.MethodPost {
		c.JSON(http.StatusMethodNotAllowed, map[string]string{"error": "Method not allowed"})
		return
	}

	body, err := io.ReadAll(c.Req.Body)
	if err != nil {
		log.Printf("[ERROR] Read body: %s", err)
		c.JSON(http.StatusBadRequest, map[string]string{"error": "read body: " + err.Error()})
		return
	}

	type ProfileSave struct {
		Profile models.Profile `json:"profile"`
		Pubkey  string         `json:"pubkey"`
	}

	req := ProfileSave{}
	if err := json.Unmarshal(body, &req.Profile); err != nil {
		log.Printf("[ERROR] profile decode: %s", err)
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "decode profile: " + err.Error()})
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("[ERROR] profile pubkey decode: %s", err)
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "decode save profile: " + err.Error()})
		return
	}

	metaEvt, err := relays.UpdateMetadataNote(req.Pubkey, req.Profile)
	if err != nil {
		log.Printf("[ERROR] update metadata: %s", err)
		c.JSON(http.StatusInternalServerError, map[string]string{"error": "update profile: " + err.Error()})
		return
	}

	if ent, err := relays.GetEntity(req.Pubkey); err == nil && ent.PubKey != "" {
		if ent.Blastr {
			relays.BlastNostrEventCh <- *metaEvt
		}
	} else {
		log.Printf("[WARN] entity not found or getEntity err: %s", err)
	}

	c.JSON(http.StatusOK, map[string]string{"message": "Profile Saved!"})
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
	items, err := relays.GetEntries()
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

func (s *Server) handleGeneratePubkeyBtn(c *router.Context) {
	metrics.CreateRequests.Inc()
	entry := s.createFeed(c.Req)

	followAction := models.FollowManagment{
		Action: models.Sync,
	}
	followManagmentCh <- followAction

	npub, _ := nip19.EncodePublicKey(s.Cfg.RelayPubkey)

	data := struct {
		RelayName    string
		PubKey       string
		Npub         string
		RelayNPubkey string
		FeedURL      string
		FeedTitle    string
		IconURL      string
		Blastr       bool
		ErrorCode    int
		Error        bool
		ErrorMessage string
		Version      string
	}{
		RelayName:    s.Cfg.RelayName,
		PubKey:       entry.BookmarkEntity.PubKey,
		Npub:         entry.BookmarkEntity.Npub,
		RelayNPubkey: npub,
		FeedURL:      entry.BookmarkEntity.FeedURL,
		FeedTitle:    entry.BookmarkEntity.FeedTitle,
		IconURL:      entry.BookmarkEntity.IconURL,
		Blastr:       entry.BookmarkEntity.Blastr,
		ErrorCode:    entry.ErrorCode,
		Error:        entry.Error,
		ErrorMessage: entry.ErrorMessage,
		Version:      config.Version,
	}

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/created.html", s.Cfg.TemplatePath)))
	err := tmpl.Execute(c.Out, data)
	if err != nil {
		log.Print("[ERROR] ", err)
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) createFeed(r *http.Request) *models.GUIEntry {
	urlParam := r.URL.Query().Get("create-feed-pubkey")

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

	sk := relays.GetPrivateKeyFromFeedUrl(feedUrl)
	publicKey, err := nostr.GetPublicKey(sk)
	if err != nil {
		guientry.ErrorCode = http.StatusInternalServerError
		guientry.Error = true
		guientry.ErrorMessage = "Bad private key: " + err.Error()
		log.Printf("[ERROR] bad private key from feed: %s", err)
		return &guientry
	}

	publicKey = strings.TrimSpace(publicKey)

	npub, err := nip19.EncodePublicKey(publicKey)
	if err != nil {
		log.Printf("[ERROR] Bad npub for feed %s with pubkey %s", feedUrl, publicKey)
		guientry.ErrorMessage = fmt.Sprintf("Bad npub for feed %s with pubkey %s", feedUrl, publicKey)
		guientry.ErrorCode = http.StatusInternalServerError
		guientry.Error = true
		return &guientry

	}

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

	localImageURL := s.Cfg.DefaultProfilePicUrl
	faviconUrl, err := yarrworker.FindFaviconURL(parsedFeed.Link, feedUrl)
	if err != nil {
		log.Print("[ERROR] FindFavicon", err)
	} else if faviconUrl != "" {
		localImageURL = faviconUrl
	}

	var validEntity models.Entity
	if _, err := models.UpdateEntity(&validEntity,
		models.WithPubKey(publicKey),
		models.WithPrivateKey(sk),
		models.WithFeedTitle(parsedFeed.Title),
		models.WithURL(feedUrl),
		models.WithNpub(npub),
		models.WithImageURL(localImageURL)); err != nil {
		guientry.ErrorCode = http.StatusBadRequest
		guientry.Error = true
		guientry.ErrorMessage = "Bad entity: " + err.Error()
		log.Printf("[ERROR] Bad entity %s", err)
		return &guientry
	}

	if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", validEntity.Npub), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.Cfg.QRCodePath, validEntity.Npub)); err != nil {
		log.Print("[ERROR]", err)
	}

	if _, err := relays.CreateMetadataNote(validEntity.PubKey, validEntity.PrivateKey,
		models.Profile{
			Name:        validEntity.FeedTitle,
			DisplayName: validEntity.FeedTitle,
			About:       parsedFeed.Description,
			Picture:     validEntity.IconURL,
			Website:     validEntity.FeedURL}); err != nil {
		log.Printf("[ERROR] creating metadata note %s", err)
	}

	lastPostTime, allPostTimes := relays.InitFeed(validEntity.PubKey, validEntity.PrivateKey, validEntity.FeedURL, parsedFeed)

	models.UpdateEntity(&validEntity,
		models.WithLastPostTime(lastPostTime),
		models.WithLastCheckedTime(time.Now().Unix()),
		models.WithAvgPostTime(relays.CalcAvgPostTime(allPostTimes)))

	if err := relays.AddEntity([]models.Entity{validEntity}); err != nil {
		log.Printf("[ERROR] feed entity %s not added to bookmark", validEntity.FeedTitle)
	}

	// only send necessary parts of Entity to gui
	guientry = models.GUIEntry{
		BookmarkEntity: models.Entity{
			PubKey:    validEntity.PubKey,
			Npub:      validEntity.Npub,
			FeedURL:   validEntity.FeedURL,
			Blastr:    validEntity.Blastr,
			FeedTitle: validEntity.FeedTitle,
			IconURL:   validEntity.IconURL,
		},
		ErrorMessage: "Success",
		Error:        false,
		ErrorCode:    0,
	}

	return &guientry
}

func handleDeleteBtn(c *router.Context) {
	metrics.DeleteRequests.Inc()
	feedPubkey := c.Req.URL.Query().Get("pubkey")

	followAction := models.FollowManagment{
		Action:       models.Delete,
		FollowEntity: models.Entity{PubKey: feedPubkey},
	}
	followManagmentCh <- followAction
	if err := relays.DeleteEntity(feedPubkey); err != nil {
		log.Printf("[ERROR] could not delete feed '%q'...Error: %s ", feedPubkey, err)
	}

	c.Out.Header().Add("HX-Refresh", "true") //will cause page to refresh
	//c.Out.WriteHeader(http.StatusNoContent) // 204 - No Content. Tells htmx to ignore any responded content and not update, does nothing, but is not an error see https://htmx.org/docs/#requests

	//tmpl := template.New("index.html")
	//tmpl.Execute(c.Out, nil)
}

func handleBlastFeed(c *router.Context) {
	feedPubkey := c.Req.URL.Query().Get("pubkey")
	currentBlastrState := c.Req.URL.Query().Get("blastr")
	newBlastrState := false
	//fmt.Printf("[DEBUG] blastr pubkey: %s\n", feedPubkey)
	//fmt.Printf("[DEBUG] blastr state: %s\n", currentBlastrState)

	//toggle the blastr state
	switch currentBlastrState {
	case "Public":
		newBlastrState = false
		currentBlastrState = "Private"
	case "Private":
		newBlastrState = true
		currentBlastrState = "Public"
	default:
		log.Printf("[ERROR] invalid blastr state: %s", currentBlastrState)
		newBlastrState = false
		currentBlastrState = "Private"
	}

	if err := relays.UpdateEntity(feedPubkey, models.WithBlastr(newBlastrState)); err != nil {
		log.Printf("[ERROR] blastr not set for feed '%s'...Error: %s", feedPubkey, err)
		c.Out.Header().Add("HX-Refresh", "true") //will cause page to refresh
		//http.Error(c.Out, "[ERROR] blastr not set", http.StatusBadRequest)
		c.Out.WriteHeader(http.StatusNoContent) // 204 - No Content. Tells htmx to ignore any responded content and not update, does nothing, but is not an error see https://htmx.org/docs/#requests
		return
	}

	updateBtn := fmt.Sprintf(`<button class="btn btn-public" hx-target="closest button" hx-confirm="Are you sure?" hx-get="./blastfeed?pubkey=%s&blastr=%s"><span class="btn-icon">🌍</span> %s</button>`, feedPubkey, currentBlastrState, currentBlastrState)
	c.Out.Write([]byte(updateBtn))
}

func (s *Server) handleImportOpml(c *router.Context) {
	metrics.ImportRequests.Inc()
	outputFileStatus := func(errMsg string) {
		htmlStr := fmt.Sprintf("<div id='progress' name='progress-bar' class='progress-bar' style='--width: 100' data-label='%s...'></div>", errMsg)
		//htmlStr := fmt.Sprintf("<div id='status-area' hx-get='./progress' hx-target='this' hx-swap='outerHTML' hx-trigger='every 600ms'><script>setProgress('Processing %s', %d);</script></div>", errMsg, 0)
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
		recentImportedEntries = s.importFeeds(doc.Body.Outlines)
	}()

	log.Print("[DEBUG] opml import started.")
	outputFileStatus("OPML import starting")
}

func (s *Server) importFeeds(opmlOutline []opml.Outline) []*models.GUIEntry {
	importedEntries := make([]*models.GUIEntry, 0)
	bookmarkEntities := make([]models.Entity, 0)

	for index, opmlEntry := range opmlOutline {
		if !helpers.IsValidHttpUrl(opmlEntry.XMLURL) {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					Npub:      "-",
					FeedURL:   opmlEntry.XMLURL,
					FeedTitle: "-"},
				ErrorMessage: "Invalid feed URL (must be in format with https or https)...",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[DEBUG] invalid feed url '%q' skipping...", opmlEntry)
			continue
		}

		discFeed, err := yarrworker.DiscoverRssFeed(opmlEntry.XMLURL)
		if err != nil || discFeed.FeedLink == "" {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					Npub:      "-",
					FeedURL:   opmlEntry.XMLURL,
					FeedTitle: "-"},
				ErrorMessage: "Could not find a feed URL in there...",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[DEBUG] Could not find a feed URL in %s", opmlEntry.XMLURL)
			continue
		}
		feedUrl := discFeed.FeedLink

		sk := relays.GetPrivateKeyFromFeedUrl(feedUrl)
		publicKey, err := nostr.GetPublicKey(sk)
		if err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					Npub:      "-",
					FeedURL:   opmlEntry.XMLURL,
					FeedTitle: "-"},
				ErrorMessage: "Bad private key",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[ERROR] feed %s bad private key: %s", feedUrl, err)
			continue
		}

		publicKey = strings.TrimSpace(publicKey)

		feedExists, err := relays.FeedExists(publicKey, sk, feedUrl)
		if feedExists {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					Npub:      "-",
					FeedURL:   opmlEntry.XMLURL,
					FeedTitle: "-"},
				ErrorMessage: "Feed already exists",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[DEBUG] feedUrl %s with pubkey %s already exists", feedUrl, publicKey)
			continue
		} else if err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					Npub:      "-",
					FeedURL:   opmlEntry.XMLURL,
					FeedTitle: "-"},
				ErrorMessage: "Could not determine if feed exists",
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[ERROR] could not determine if feedUrl %s with pubkey %s exists", feedUrl, publicKey)
			continue
		}

		npub, err := nip19.EncodePublicKey(publicKey)
		if err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					Npub:      "-",
					FeedURL:   opmlEntry.XMLURL,
					FeedTitle: "-"},
				ErrorMessage: "Bad npub: " + err.Error(),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[ERROR] can not create npub %s", err)
			continue
		}

		parsedFeed, err := relays.ParseFeedForUrl(feedUrl)
		if err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					FeedURL:   opmlEntry.XMLURL,
					Npub:      "-",
					FeedTitle: "-"},
				ErrorMessage: "Can not parse feed: " + err.Error(),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[ERROR] can not parse feed %s", err)
			continue
		}

		localImageURL := s.Cfg.DefaultProfilePicUrl
		faviconUrl, err := yarrworker.FindFaviconURL(parsedFeed.Link, feedUrl)
		if err != nil {
			log.Print("[ERROR] FindFavicon", err)
		} else if faviconUrl != "" {
			localImageURL = faviconUrl
		}

		var validEntity models.Entity
		if _, err := models.UpdateEntity(&validEntity,
			models.WithPubKey(publicKey),
			models.WithPrivateKey(sk),
			models.WithFeedTitle(parsedFeed.Title),
			models.WithURL(feedUrl),
			models.WithNpub(npub),
			models.WithImageURL(localImageURL)); err != nil {
			importedEntries = append(importedEntries, &models.GUIEntry{
				BookmarkEntity: models.Entity{
					Npub:      "-",
					FeedURL:   opmlEntry.XMLURL,
					FeedTitle: "-"},
				ErrorMessage: "Bad entity: " + err.Error(),
				Error:        true,
				ErrorCode:    http.StatusBadRequest,
			})
			importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
			log.Printf("[ERROR] bad entity %s", err)
			continue
		}

		if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", validEntity.Npub), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.Cfg.QRCodePath, validEntity.Npub)); err != nil {
			log.Print("[ERROR] ", err)
		}

		if _, err := relays.CreateMetadataNote(publicKey, sk,
			models.Profile{
				Name:        validEntity.FeedTitle,
				DisplayName: validEntity.FeedTitle,
				About:       parsedFeed.Description,
				Picture:     validEntity.IconURL,
				Website:     validEntity.FeedURL}); err != nil {
			log.Printf("[ERROR] creating metadata note %s", err)
		}

		lastPostTime, allPostTimes := relays.InitFeed(publicKey, sk, feedUrl, parsedFeed)

		models.UpdateEntity(&validEntity,
			models.WithLastPostTime(lastPostTime),
			models.WithLastCheckedTime(time.Now().Unix()),
			models.WithAvgPostTime(relays.CalcAvgPostTime(allPostTimes)))

		bookmarkEntities = append(bookmarkEntities, validEntity)

		// only send necessary parts of Entity to gui
		guiEntry := models.GUIEntry{
			BookmarkEntity: models.Entity{
				Npub:      validEntity.Npub,
				FeedURL:   validEntity.FeedURL,
				FeedTitle: validEntity.FeedTitle,
			},
			ErrorMessage: "Success",
			Error:        false,
			ErrorCode:    0,
		}

		importedEntries = append(importedEntries, &guiEntry)
		importProgressCh <- ImportProgressStruct{EntryIndex: index, TotalEntries: len(opmlOutline)}
	}

	if err := relays.AddEntity(bookmarkEntities); err != nil {
		log.Printf("[ERROR] adding feed entities: %s", err)
	}

	//update kind 3 event
	followAction := models.FollowManagment{
		Action: models.Sync,
	}
	followManagmentCh <- followAction

	return importedEntries
}

func handleImportProgress(c *router.Context) {
	importedURL := <-importProgressCh
	progressPct := ((float32(importedURL.EntryIndex) + 1.0) / float32(importedURL.TotalEntries)) * 100.0

	if importedURL.EntryIndex+1 < importedURL.TotalEntries {
		htmlStr := fmt.Sprintf("<div id='status-area' hx-get='./progress' hx-target='this' hx-swap='outerHTML' hx-trigger='every 600ms'><script>setProgress('Processing %d of %d', %f);</script></div>", importedURL.EntryIndex+1, importedURL.TotalEntries, progressPct)
		tmpl, _ := template.New("t").Parse(htmlStr)
		tmpl.Execute(c.Out, nil)
	} else {
		htmlStr := fmt.Sprintf("<div id='status-area' hx-get='./progress' hx-target='this' hx-swap='outerHTML' hx-trigger='change from:#opml-import-form' hx-sync='#opml-file: queue first'><script>setProgress('Processing %d of %d', %f);</script></div>", importedURL.EntryIndex+1, importedURL.TotalEntries, progressPct)
		//htmlStr := fmt.Sprintf("<div id='status-area' hx-get='./progress' hx-target='this' hx-swap='outerHTML' hx-trigger='change from:#opml-import-form' hx-sync='#opml-file: queue first'><a href='./'>Refresh</a>..or..<a href='./detail'>Details</a><script>setProgress('Processing %d of %d', %f);</script></div>", importedURL.EntryIndex+1, importedURL.TotalEntries, progressPct)
		tmpl, _ := template.New("t").Parse(htmlStr)
		tmpl.Execute(c.Out, nil)
	}
}

func (s *Server) handleImportDetail(c *router.Context) {

	numBadFeeds := 0
	for _, feed := range recentImportedEntries {
		if feed.Error {
			numBadFeeds++
		}
	}

	npub, _ := nip19.EncodePublicKey(s.Cfg.RelayPubkey)

	results := struct {
		RelayName    string
		RelayNPubkey string
		Feeds        []*models.GUIEntry
		GoodFeeds    int
		BadFeeds     int
		Version      string
	}{
		RelayName:    s.Cfg.RelayName,
		RelayNPubkey: npub,
		Feeds:        recentImportedEntries,
		GoodFeeds:    len(recentImportedEntries) - numBadFeeds,
		BadFeeds:     numBadFeeds,
		Version:      config.Version,
	}

	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/imported.html", s.Cfg.TemplatePath)))

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

	data, err := relays.GetEntities()
	if err != nil {
		log.Printf("[ERROR] no entities found")
	}

	for _, feed := range data {
		rssOMPL.Body.Outlines = append(rssOMPL.Body.Outlines, opml.Outline{
			Type:    "rss",
			Text:    feed.PubKey,
			Title:   feed.FeedTitle,
			XMLURL:  feed.FeedURL,
			HTMLURL: feed.FeedURL,
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

func (s *Server) handleSearchBtn(c *router.Context) {

	/* 	funcs := template.FuncMap{
		"shortURL": func(urlLink string) string {
			u, err := url.Parse(urlLink)
			if err != nil {
				log.Printf("[ERROR] shortURL: %s", err.Error())
				return urlLink
			}
			return strings.TrimPrefix(u.Host, "www.")
		},
	} */

	//tmpl := template.Must(template.New("search.html").Funcs(funcs).ParseFiles(fmt.Sprintf("%s/search.html", s.Cfg.TemplatePath)))
	tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%s/search.html", s.Cfg.TemplatePath)))
	metrics.SearchRequests.Inc()
	query := c.Req.URL.Query().Get("search-query")
	//fmt.Printf("[DEBUG] search-query: %s\n", query)
	npub, _ := nip19.EncodePublicKey(s.Cfg.RelayPubkey)

	if query == "" || len(query) <= 4 {

		errorData := struct {
			RelayName     string
			RelayNPubkey  string
			Count         uint64
			FilteredCount uint64
			Entries       []models.GUIEntry
			Error         bool
			ErrorMessage  string
			Version       string
		}{
			RelayName:     s.Cfg.RelayName,
			RelayNPubkey:  npub,
			Count:         0,
			FilteredCount: 0,
			Entries:       nil,
			Error:         true,
			ErrorMessage:  "Please enter more than 5 characters to search!",
			Version:       config.Version,
		}

		if err := tmpl.Execute(c.Out, errorData); err != nil {
			log.Print("[ERROR] ", err)
			http.Error(c.Out, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	savedEntries, err := relays.GetEntries()
	if err != nil {
		http.Error(c.Out, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]models.GUIEntry, 0)
	for _, entry := range savedEntries {
		if strings.Contains(strings.ToLower(entry.BookmarkEntity.FeedURL), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(entry.BookmarkEntity.FeedTitle), strings.ToLower(query)) {
			items = append(items, entry)
		}
	}

	data := struct {
		RelayName     string
		RelayNPubkey  string
		Count         uint64
		FilteredCount uint64
		Entries       []models.GUIEntry
		Error         bool
		ErrorMessage  string
		Version       string
	}{
		RelayName:     s.Cfg.RelayName,
		RelayNPubkey:  npub,
		Count:         uint64(len(savedEntries)),
		FilteredCount: uint64(len(items)),
		Entries:       items,
		Error:         false,
		ErrorMessage:  "",
		Version:       config.Version,
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
