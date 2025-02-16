// Parser for RSS versions:
// - 0.90
// - 1.0
package yarrparser

import (
	"encoding/xml"
	"io"
)

type rdfFeed struct {
	XMLName xml.Name  `xml:"RDF"`
	Title   string    `xml:"channel>title"`
	Link    string    `xml:"channel>link"`
	Items   []rdfItem `xml:"item"`
}

type rdfItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`

	DublinCoreDate string `xml:"http://purl.org/dc/elements/1.1/ date"`
	ContentEncoded string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

func ParseRDF(r io.Reader) (*Feed, error) {
	srcfeed := rdfFeed{}

	decoder := xmlDecoder(r)
	if err := decoder.Decode(&srcfeed); err != nil {
		return nil, err
	}

	dstfeed := &Feed{
		Title:   srcfeed.Title,
		SiteURL: srcfeed.Link,
	}
	for _, srcitem := range srcfeed.Items {
		dstfeed.Items = append(dstfeed.Items, Item{
			GUID:    srcitem.Link,
			URL:     srcitem.Link,
			Date:    dateParse(srcitem.DublinCoreDate),
			Title:   srcitem.Title,
			Content: firstNonEmpty(srcitem.ContentEncoded, srcitem.Description),
		})
	}
	return dstfeed, nil
}
