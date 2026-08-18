package graph

import (
	"testing"
)

func TestNewGraph(t *testing.T) {
	g := NewGraph()
	if g == nil || g.Nodes == nil {
		t.Fatal("NewGraph failed")
	}
}

func TestAddNode(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{Name: "A"})
	g.AddNode(&Node{Name: "B"})
	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}
}

func TestAddEdge(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{Name: "A"})
	g.AddNode(&Node{Name: "B"})
	g.AddEdge(Edge{From: "A", To: "B"})
	if len(g.Edges) != 1 || g.Edges[0].From != "A" {
		t.Error("AddEdge failed")
	}
}

func TestTopoSort_Simple(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{Name: "A"})
	g.AddNode(&Node{Name: "B"})
	g.AddEdge(Edge{From: "A", To: "B"})
	order, err := g.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "A" || order[1] != "B" {
		t.Errorf("expected [A B], got %v", order)
	}
}

func TestTopoSort_Diamond(t *testing.T) {
	g := NewGraph()
	for _, n := range []string{"A", "B", "C", "D"} {
		g.AddNode(&Node{Name: n})
	}
	g.AddEdge(Edge{From: "A", To: "B"})
	g.AddEdge(Edge{From: "A", To: "C"})
	g.AddEdge(Edge{From: "B", To: "D"})
	g.AddEdge(Edge{From: "C", To: "D"})

	order, err := g.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 4 || order[0] != "A" || order[3] != "D" {
		t.Errorf("expected A first and D last, got %v", order)
	}
}

func TestTopoSort_Cycle(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{Name: "A"})
	g.AddNode(&Node{Name: "B"})
	g.AddEdge(Edge{From: "A", To: "B"})
	g.AddEdge(Edge{From: "B", To: "A"})

	_, err := g.TopoSort()
	if err == nil {
		t.Error("expected error for cycle")
	}
}

func TestTopoSort_Empty(t *testing.T) {
	g := NewGraph()
	order, err := g.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 0 {
		t.Errorf("expected empty, got %v", order)
	}
}

func TestTopoSort_Independent(t *testing.T) {
	g := NewGraph()
	g.AddNode(&Node{Name: "A"})
	g.AddNode(&Node{Name: "B"})
	g.AddNode(&Node{Name: "C"})

	order, err := g.TopoSort()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 {
		t.Errorf("expected 3, got %d", len(order))
	}
}
