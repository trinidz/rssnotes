package main

import (
	"log"
	"net/http"
	"os"

	"rssnotes/internal/config"
	"rssnotes/server"

	"github.com/hashicorp/logutils"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Panic("[ERROR] No .env file found!")
	}

	var c config.C

	if err := envconfig.Process("", &c); err != nil {
		log.Panicf("[ERROR] couldn't process envconfig: %s", err)
		return
	}

	log.SetFlags(log.Lshortfile | log.LstdFlags)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(c.LogLevel),
		Writer:   os.Stderr,
	}
	log.SetOutput(filter)

	srvr := server.NewServer(c)

	log.Printf("[WARN] listening on 0.0.0.0:%s%s\n", srvr.Cfg.Port, srvr.GetAddr().Path)
	log.Printf("[INFO] public url (RELAY_URL) set to %s\n", srvr.GetAddr().Scheme+"://"+srvr.GetAddr().Host+srvr.GetAddr().Path)
	if err := http.ListenAndServe(":"+srvr.Cfg.Port, srvr.Serve()); err != nil {
		log.Panicf("[FATAL] ListenAndServe error %s", err)
	}
}
