package main

import (
	"encoding/xml"
	"io"
	"net/http"
	"os"
)

type FollowListAction int

const (
	Sync FollowListAction = iota
	Add
	Delete
)

type FollowManagment struct {
	Action       FollowListAction
	FollowEntity Entity
}

type ImportProgressStruct struct {
	entryIndex   int
	totalEntries int
}

type Entity struct {
	PubKey          string
	PrivateKey      string
	URL             string
	ImageURL        string
	LastPostTime    int64
	AvgPostTime     int64
	LastCheckedTime int64
}

type GUIEntry struct {
	BookmarkEntity Entity
	NPubKey        string
	Error          bool
	ErrorMessage   string
	ErrorCode      int
}

// Nostr Kind-0
type KindProfileMetadata struct {
	About   string
	Picture string
	Name    string
}

// OpmlOutline holds all information about an outline.
type OpmlOutline struct {
	Outlines     []OpmlOutline `xml:"outline"`
	Text         string        `xml:"text,attr"`
	Type         string        `xml:"type,attr,omitempty"`
	IsComment    string        `xml:"isComment,attr,omitempty"`
	IsBreakpoint string        `xml:"isBreakpoint,attr,omitempty"`
	Created      string        `xml:"created,attr,omitempty"`
	Category     string        `xml:"category,attr,omitempty"`
	XMLURL       string        `xml:"xmlUrl,attr,omitempty"`
	HTMLURL      string        `xml:"htmlUrl,attr,omitempty"`
	URL          string        `xml:"url,attr,omitempty"`
	Language     string        `xml:"language,attr,omitempty"`
	Title        string        `xml:"title,attr,omitempty"`
	Version      string        `xml:"version,attr,omitempty"`
	Description  string        `xml:"description,attr,omitempty"`
}

// OpmlBody is the parent structure of all outlines.
type OpmlBody struct {
	Outlines []OpmlOutline `xml:"outline"`
}

// OpmlHead holds some meta information about the document.
type OpmlHead struct {
	Title           string `xml:"title"`
	DateCreated     string `xml:"dateCreated,omitempty"`
	DateModified    string `xml:"dateModified,omitempty"`
	OwnerName       string `xml:"ownerName,omitempty"`
	OwnerEmail      string `xml:"ownerEmail,omitempty"`
	OwnerID         string `xml:"ownerId,omitempty"`
	Docs            string `xml:"docs,omitempty"`
	ExpansionState  string `xml:"expansionState,omitempty"`
	VertScrollState string `xml:"vertScrollState,omitempty"`
	WindowTop       string `xml:"windowTop,omitempty"`
	WindowBottom    string `xml:"windowBottom,omitempty"`
	WindowLeft      string `xml:"windowLeft,omitempty"`
	WindowRight     string `xml:"windowRight,omitempty"`
}

// OPML is the root node of an OPML document. It only has a single required
// attribute: the version.
type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    OpmlHead `xml:"head"`
	Body    OpmlBody `xml:"body"`
}

// XML exports the OPML document to a XML string.
func (doc OPML) XML() (string, error) {
	b, err := xml.MarshalIndent(doc, "", "\t")
	return xml.Header + string(b), err
}

// NewOPML creates a new OPML structure from a slice of bytes.
func NewOPML(b []byte) (*OPML, error) {
	var root OPML
	err := xml.Unmarshal(b, &root)
	if err != nil {
		return nil, err
	}

	return &root, nil
}

// NewOPMLFromURL creates a new OPML structure from an URL.
func NewOPMLFromURL(url string) (*OPML, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return NewOPML(b)
}

// NewOPMLFromFile creates a new OPML structure from a file.
func NewOPMLFromFile(filePath string) (*OPML, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return NewOPML(b)
}
