package models

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
)

type Entity struct {
	PubKey          string
	Npub            string
	PrivateKey      string
	FeedURL         string
	IconURL         string
	FeedTitle       string
	LastPostTime    int64
	AvgPostTime     int64
	LastCheckedTime int64
	Blastr          bool
}

type GUIEntry struct {
	BookmarkEntity Entity
	Error          bool
	ErrorMessage   string
	ErrorCode      int
}

// Option is a function that applies a modification to an Entity.
// It now returns an error if validation fails.
type Option func(*Entity) error

// Helper to validate hex strings (64 chars, lowercase)
func validateHex(s string, fieldName string) error {
	if len(s) != 64 {
		return fmt.Errorf("invalid %s: expected 64 hex characters, got %d", fieldName, len(s))
	}
	// Check for valid hex characters
	for _, r := range strings.ToLower(s) {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return fmt.Errorf("invalid %s: contains non-hex character '%c'", fieldName, r)
		}
	}
	return nil
}

// validateImageFileType checks if the file extension corresponds to a valid image MIME type.
// It returns true for common extensions like .jpg, .png, .gif, .webp, .svg, etc.
func validateImageFileType(absPath string) bool {
	switch ext := strings.ToLower(filepath.Ext(absPath)); ext {
	case ".jpg", ".jpeg", ".svg", ".png", ".webp", ".gif", ".ico":
		return true

	default:
		log.Printf("[DEBUG] Invalid image file ext: %s ", ext)
	}

	return false
}

// WithPubKey sets the public key with validation.
func WithPubKey(pk string) Option {
	return func(e *Entity) error {
		if pk == "" {
			return errors.New("pubkey cannot be empty")
		}
		if err := validateHex(pk, "pubkey"); err != nil {
			return err
		}
		e.PubKey = pk
		return nil
	}
}

// WithNpub sets the npub with basic validation.
func WithNpub(np string) Option {
	return func(e *Entity) error {
		if np == "" {
			return errors.New("npub cannot be empty")
		}
		if !strings.HasPrefix(np, "npub1") {
			return errors.New("npub must start with 'npub1'")
		}
		e.Npub = np
		return nil
	}
}

// WithPrivateKey sets the private key.
func WithPrivateKey(pk string) Option {
	return func(e *Entity) error {
		if pk == "" {
			return errors.New("private key cannot be empty")
		}
		// Optional: Validate hex length for private keys too
		if err := validateHex(pk, "private key"); err != nil {
			return err
		}
		e.PrivateKey = pk
		return nil
	}
}

// WithURL sets the URL with basic validation.
func WithURL(url string) Option {
	return func(e *Entity) error {
		if url == "" {
			return errors.New("URL cannot be empty")
		}
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return errors.New("URL must start with http:// or https://")
		}
		e.FeedURL = url
		return nil
	}
}

// WithImageURL sets the image URL.
func WithImageURL(url string) Option {
	return func(e *Entity) error {
		if url == "" {
			return errors.New("image URL cannot be empty")
		}
		if !validateImageFileType(url) && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			if !validateImageFileType(url) {
				return errors.New("image type must be .png, .svg etc...")
			}
			return errors.New("image URL  must start with http:// or https://")
		}

		e.IconURL = url
		return nil
	}
}

// WithFeedTitle sets the feed title.
func WithFeedTitle(title string) Option {
	return func(e *Entity) error {
		if title == "" {
			return errors.New("feed title cannot be empty")
		}
		e.FeedTitle = title
		return nil
	}
}

// WithLastPostTime sets the last post time.
func WithLastPostTime(t int64) Option {
	return func(e *Entity) error {
		if t < 0 {
			return errors.New("last post time cannot be negative")
		}
		e.LastPostTime = t
		return nil
	}
}

// WithAvgPostTime sets the average post time.
func WithAvgPostTime(t int64) Option {
	return func(e *Entity) error {
		if t < 0 {
			return errors.New("average post time cannot be negative")
		}
		e.AvgPostTime = t
		return nil
	}
}

// WithLastCheckedTime sets the last checked time.
func WithLastCheckedTime(t int64) Option {
	return func(e *Entity) error {
		if t < 0 {
			return errors.New("last checked time cannot be negative")
		}
		e.LastCheckedTime = t
		return nil
	}
}

// WithBlastr sets the Blastr flag. No validation needed for bool.
func WithBlastr(b bool) Option {
	return func(e *Entity) error {
		e.Blastr = b
		return nil
	}
}

// UpdateEntity applies a set of options to an existing Entity.
// It returns the modified entity and an error if any validation failed.
func UpdateEntity(entity *Entity, opts ...Option) (*Entity, error) {
	if entity == nil {
		return nil, errors.New("entity cannot be nil")
	}

	var errs []error
	for _, opt := range opts {
		if err := opt(entity); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		// Return all validation errors combined or the first one

		// Here we return the first error
		//return entity, fmt.Errorf("update failed: %v", errs[0])

		// Join errors. Go 1.20+ uses errors.Join for automatic formatting:
		return entity, errors.Join(errs...)
	}

	return entity, nil
}
