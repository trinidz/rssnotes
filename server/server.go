package server

import (
	"log"
	"net/http"
	"net/url"

	"rssnotes/internal/config"
	"strings"
)

type Server struct {
	addr string
	Cfg  *config.C
	// Feeds *rssfeeds.RssFeedStack
}

func NewServer(cfg *config.C) *Server {
	serverConfig := cfg
	if serverConfig.RelayBasepath != "" {
		serverConfig.RelayBasepath = "/" + strings.Trim(serverConfig.RelayBasepath, "/")
	}

	return &Server{
		Cfg: serverConfig,
		//Feeds: rssfd,
	}
}

func (s *Server) Serve() http.Handler {
	if s.Cfg == nil {
		log.Panic("[ERROR] Server() envconfig or KhatruRelay not set")
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

// func (s *Server) Start() http.Handler {
// 	s.Serve().ServeHTTP(w, r)
// }
