package server

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rssnotes/internal/config"
	"rssnotes/internal/models"
	"rssnotes/internal/relays"

	"github.com/fiatjaf/khatru"
)

var (
	tickerUpdateFeeds    *time.Ticker
	tickerDeleteOldNotes *time.Ticker
	quitChannel          = make(chan struct{})
	followManagmentCh    = make(chan models.FollowManagment)
)

type Server struct {
	Cfg   *config.C
	relay *khatru.Relay
}

func NewServer(cfg config.C) *Server {
	if cfg.RelayBasepath != "" {
		cfg.RelayBasepath = "/" + strings.Trim(cfg.RelayBasepath, "/")
	}

	rly := relays.InitRelay(cfg)

	tickerUpdateFeeds = time.NewTicker(time.Duration(cfg.FeedItemsRefreshMinutes) * time.Minute)
	tickerDeleteOldNotes = time.NewTicker(time.Duration(24) * time.Hour)

	go updateRssNotesState()
	go relays.BlastWorker(relays.BlastNostrEventCh)

	return &Server{
		Cfg:   &cfg,
		relay: rly,
	}
}

func (s *Server) Serve() http.Handler {
	if s.Cfg == nil {
		log.Panic("[FATAL] Server() envconfig not set")
		return nil
	}
	return s.handler()
}

func (s *Server) GetAddr() *url.URL {
	u, err := url.Parse(s.Cfg.RelayURL + s.Cfg.RelayBasepath)
	if err != nil {
		log.Panicf("[FATAL] %s bad RelayURL %s or Basepath %s in env file.", err, s.Cfg.RelayURL, s.Cfg.RelayBasepath)
	}

	//log.Printf("[DEBUG] public url %s", u.Scheme+"://"+u.Host+u.Path)
	return u
}

func updateRssNotesState() {
	for {
		select {
		case followAction := <-followManagmentCh:
			relays.UpdateFollowListEvent(followAction)
		case <-tickerUpdateFeeds.C:
			relays.CheckAllFeeds()
		case <-tickerDeleteOldNotes.C:
			relays.DeleteOldKindTextNoteEvents()
		case <-quitChannel:
			tickerUpdateFeeds.Stop()
			tickerDeleteOldNotes.Stop()
			return
		}
	}
}
