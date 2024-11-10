package main

import (
	"context"
	"log"
	"slices"

	"github.com/nbd-wtf/go-nostr"
)

func policyEventReadOnly(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
	log.Printf("[DEBUG] %s tried to write a kind %d to readonly relay", event.PubKey, event.Kind)
	return true, "restricted: this is a readonly relay"
}

func policyFilterBookmark(_ context.Context, filter nostr.Filter) (reject bool, msg string) {
	if slices.Contains(filter.Kinds, 10003) {
		log.Printf("[DEBUG] kind 10003 req not allowed: %d\n", filter.Kinds)
		return true, "restricted: can not read kind 10003 from this relay"
	}
	return false, ""
}
