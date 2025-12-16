package main

import (
	"fmt"
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

	logFile, err := os.OpenFile(c.LogfilePath, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panicf("[FATAL] Logfile error: %s", err)
	}
	defer logFile.Close()

	log.SetFlags(log.Lshortfile | log.LstdFlags)

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"},
		MinLevel: logutils.LogLevel(c.LogLevel),
		Writer:   logFile,
	}
	log.SetOutput(filter)

	srvr := server.NewServer(c)

	fmt.Printf("listeniing on 0.0.0.0:%s%s\n", srvr.Cfg.Port, srvr.GetAddr().Path)
	fmt.Printf("public url at %s\n", srvr.GetAddr().Scheme+"://"+srvr.GetAddr().Host+srvr.GetAddr().Path)
	if err := http.ListenAndServe(":"+c.Port, srvr.Serve()); err != nil {
		fmt.Printf("ListenAndServe error %s", err)
		log.Panicf("[FATAL] ListenAndServe error %s", err)
	}
}
