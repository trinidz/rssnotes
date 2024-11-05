package main

import (
	"context"
	"log"
	"slices"

	"github.com/nbd-wtf/go-nostr"
)

// only accept events from authors on the whitelist
func policyEventWhitelist(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
	if isPublicKeyInWhitelist(event.PubKey) {
		return false, "" //this person/event is whitelisted so allow
	}
	log.Printf("[DEBUB] %s NOT in whitelist\n", event.PubKey)
	return true, "NOT in whitelist" // anyone else can NOT write events
}

// only accept filters/reqs from clients that auth
func policyFilterBookmark(_ context.Context, filter nostr.Filter) (reject bool, msg string) {
	if slices.Contains(filter.Kinds, 10003) {
		log.Printf("[DEBUG] kind 10003 req not allowed: %d\n", filter.Kinds)
		return true, "can not read kind 10003 from this relay"
	}
	return false, ""
}

/* // only accept filters/reqs from clients that auth
func policyFilterAuth(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
	if pubkey := khatru.GetAuthed(ctx); pubkey != "" {
		log.Printf("req from %s\n", pubkey)
		return false, ""
	}
	fmt.Printf("Failed to auth: %s\n", filter)
	return true, "auth-required: only authenticated users can read from this relay"
	// (this will cause an AUTH message to be sent and then a CLOSED message such that clients can
	// authenticate and then request again)
} */
