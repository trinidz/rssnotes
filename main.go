package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/fiatjaf/eventstore/badger"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/policies"
	"github.com/hashicorp/logutils"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/mmcdole/gofeed"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/skip2/go-qrcode"
)

type Settings struct {
	RelayName        string `envconfig:"RELAY_NAME" default:"rssnotes"`
	RelayURL         string `envconfig:"RELAY_URL" required:"true"`
	RelayPubkey      string `envconfig:"RELAY_PUBKEY" required:"true"`
	RelayPrivkey     string `envconfig:"RELAY_PRIVKEY" required:"true"`
	RelayDescription string `envconfig:"RELAY_DESCRIPTION" default:"An rss to nostr relay."`
	RelayContact     string `envconfig:"RELAY_CONTACT" default:"example@example.com"`
	RelayIcon        string `envconfig:"RELAY_ICON" default:"https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/png/commafeed.png"`
	RandomSecret     string `envconfig:"RANDOM_SECRET" required:"true"`
	OwnerPubkey      string `envconfig:"OWNER_PUBKEY"`
	Version          string `envconfig:"VERSION" default:"0.0.4"`

	LogLevel       string `envconfig:"LOG_LEVEL" default:"WARN"`
	Port           string `envconfig:"PORT" default:"3334"`
	DatabasePath   string `envconfig:"DATABASE_PATH" default:"./db/rssnotes"`
	FrensdataPath  string `envconfig:"FRENSDATA_PATH" default:"./frens.json"`
	SeedRelaysPath string `envconfig:"SEED_RELAYS_PATH" default:"./seedrelays.json"`
	LogfilePath    string `envconfig:"LOGFILE_PATH" default:"./logfile.log"`
	TemplatePath   string `envconfig:"TEMPLATE_PATH" default:"./web/templates"`
	StaticPath     string `envconfig:"STATIC_PATH" default:"./web/assets"`
	QRCodePath     string `envconfig:"QRCODE_PATH" default:"./web/assets/qrcodes"`

	RsslayTagKey            string `envconfig:"RSSLAY_TAG_KEY" default:"rsslay"`
	DefaultProfilePicUrl    string `envconfig:"DEFAULT_PROFILE_PICTURE_URL" default:"/assets/static/mstile-150x150.png"`
	DeleteFailingFeeds      bool   `envconfig:"DELETE_FAILIING_FEEDS" required:"false"`
	MaxContentLength        int    `envconfig:"MAX_CONTENT_LENGTH" default:"250"`
	FeedItemsRefreshMinutes int    `envconfig:"FEED_ITEMS_REFRESH_MINUTES" default:"30"`
	FeedMetadataRefreshDays int    `envconfig:"METADATA_REFRESH_DAYS" default:"7"`
	MaxNoteAgeDays          int    `envconfig:"MAX_NOTE_AGE_DAYS" default:"0"`
}

var (
	s Settings
	//db = lmdb.LMDBBackend{}
	db                   = badger.BadgerBackend{}
	relay                = khatru.NewRelay()
	followManagmentCh    = make(chan FollowManagment)
	importProgressCh     = make(chan ImportProgressStruct)
	pool                 *nostr.SimplePool
	seedRelays           []string
	tickerUpdateFeeds    *time.Ticker
	tickerDeleteOldNotes *time.Ticker
	quitChannel          = make(chan struct{})
)

func main() {
	ctx := context.Background()
	pool = nostr.NewSimplePool(ctx)

	if err := godotenv.Load(); err != nil {
		log.Panic("[ERROR] No .env file found!")
	}

	if err := envconfig.Process("", &s); err != nil {
		log.Panicf("[ERROR] couldn't process envconfig: %s", err)
		return
	}

	logFile, err := os.OpenFile(s.LogfilePath, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panicf("[FATAL] Logfile error: %s", err)
	}
	defer logFile.Close()

	log.SetFlags(log.Lshortfile | log.LstdFlags)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(s.LogLevel),
		Writer:   logFile,
	}
	log.SetOutput(filter)

	seedRelays = GetRelayListFromFile(s.SeedRelaysPath)
	if len(seedRelays) == 0 {
		log.Panic("[FATAL] 0 seed relays; need to set relays")
		return
	}

	// basic properties returned on the NIP-11 endpoint
	relay.Info.Name = s.RelayName
	relay.Info.PubKey = s.RelayPubkey
	relay.Info.Description = s.RelayDescription
	relay.Info.Contact = s.RelayContact
	relay.Info.Icon = s.RelayIcon

	db.Path = s.DatabasePath
	//os.MkdirAll(db.Path, 0755)
	if err := db.Init(); err != nil {
		log.Panicf("[FATAL] db init: %s", err)
		return
	}
	defer db.Close()

	relay.StoreEvent = append(relay.StoreEvent, db.SaveEvent)
	relay.QueryEvents = append(relay.QueryEvents, db.QueryEvents)
	relay.CountEvents = append(relay.CountEvents, db.CountEvents)
	relay.DeleteEvent = append(relay.DeleteEvent, db.DeleteEvent)

	relay.RejectEvent = append(relay.RejectEvent,
		policyEventReadOnly,
	)

	relay.RejectFilter = append(relay.RejectFilter,
		policies.NoComplexFilters,
		policyFilterBookmark,
	)

	npub, err := nip19.EncodePublicKey(s.RelayPubkey)
	if err != nil {
		log.Printf("[ERROR] %s", err)
	}

	if err := createMetadataNote(s.RelayPubkey, s.RelayPrivkey, &gofeed.Feed{Title: s.RelayName, Description: s.RelayDescription}, s.DefaultProfilePicUrl); err != nil {
		log.Print("[ERROR] ", err)
	}

	if _, err := os.Stat(fmt.Sprintf("%s/%s.png", s.QRCodePath, npub)); errors.Is(err, os.ErrNotExist) {
		if err := qrcode.WriteFile(fmt.Sprintf("nostr:%s", npub), qrcode.Low, 128, fmt.Sprintf("%s/%s.png", s.QRCodePath, npub)); err != nil {
			log.Print("[ERROR] creating relay QR code", err)
		}
	}

	tickerUpdateFeeds = time.NewTicker(time.Duration(s.FeedItemsRefreshMinutes) * time.Minute)
	tickerDeleteOldNotes = time.NewTicker(time.Duration(24) * time.Hour)

	go updateRssNotesState()

	mux := relay.Router()
	mux.HandleFunc("/", handleFrontpage)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(s.StaticPath))))
	mux.HandleFunc("GET /create", (func(w http.ResponseWriter, r *http.Request) {
		handleCreateFeed(w, r, &s.RandomSecret)
	}))
	mux.HandleFunc("GET /search", handleSearch)
	mux.HandleFunc("POST /import", handleImportOpml)
	mux.HandleFunc("GET /progress", handleImportProgress)
	mux.HandleFunc("GET /detail", handleImportDetail)
	mux.HandleFunc("GET /export", handleExportOpml)
	mux.HandleFunc("GET /delete", handleDeleteFeed)

	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /log", handleLog)

	fmt.Printf("running on 0.0.0.0:%s\n", s.Port)
	if err := http.ListenAndServe(":"+s.Port, relay); err != nil {
		fmt.Printf("ListenAndServe error %s", err)
		log.Panicf("ListenAndServe error %s", err)
	}
}
