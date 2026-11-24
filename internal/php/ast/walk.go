package ast

func Walk(n Node, enter func(Node) bool) {
	if !enter(n) {
		return
	}
	Children(n, func(c Node) { Walk(c, enter) })
}
