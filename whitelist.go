package main

import (
	"encoding/json"
	"errors"
	"os"
)

type Whitelist map[string]string // { [user_pubkey]: [user_name] }

func loadWhitelist() error {
	b, err := os.ReadFile(s.FrensdataPath)
	if err != nil {
		// If the whitelist file does not exist, with RELAY_PUBKEY
		if errors.Is(err, os.ErrNotExist) {
			whitelist[s.RelayPubkey] = ""
			jsonBytes, err := json.Marshal(&whitelist)
			if err != nil {
				return err
			}
			if err := os.WriteFile(s.FrensdataPath, jsonBytes, 0644); err != nil {
				return err
			}
			return nil
		} else {
			return err
		}
	}

	if err := json.Unmarshal(b, &whitelist); err != nil {
		return err
	}

	return nil
}

func isPublicKeyInWhitelist(pubkey string) bool {
	_, ok := whitelist[pubkey]
	return ok
}
