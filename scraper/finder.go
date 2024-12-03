package scraper

import (
	"strings"

	"rssnotes/htmlutil"

	"golang.org/x/net/html"
)

func FindIcons(body string, base string) []string {
	icons := make([]string, 0)

	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return icons
	}

	// css: link[rel=icon]
	isLink := func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "link"
	}
	for _, node := range htmlutil.FindNodes(doc, isLink) {
		rels := strings.Split(htmlutil.Attr(node, "rel"), " ")
		for _, rel := range rels {
			if strings.EqualFold(rel, "icon") {
				icons = append(icons, htmlutil.AbsoluteUrl(htmlutil.Attr(node, "href"), base))
			}
		}
	}
	return icons
}
