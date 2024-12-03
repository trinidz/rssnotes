package htmlutil

import (
	"golang.org/x/net/html"
)

func FindNodes(node *html.Node, match func(*html.Node) bool) []*html.Node {
	nodes := make([]*html.Node, 0)

	queue := make([]*html.Node, 0)
	queue = append(queue, node)
	for len(queue) > 0 {
		var n *html.Node
		n, queue = queue[0], queue[1:]
		if match(n) {
			nodes = append(nodes, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			queue = append(queue, c)
		}
	}
	return nodes
}
