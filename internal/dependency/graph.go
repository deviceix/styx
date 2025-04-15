package dependency

import (
	"errors"
	"fmt"
)

// NodeType represents the type of node in the build graph
type NodeType int

const (
	NodeTypeSource NodeType = iota
	NodeTypeHeader
	NodeTypeObject
	NodeTypeLibrary
	NodeTypeExecutable
)

// Node represents a node in the build graph
type Node struct {
	ID           string
	Type         NodeType
	Path         string
	Hash         string
	Timestamp    int64
	Dependencies []*Node
	CommandHash  string
}

// Graph represents a directed acyclic graph of build dependencies
type Graph struct {
	Nodes       map[string]*Node
	EntryPoints []*Node
}

// NewGraph creates a new empty dependency graph
func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
	}
}

// AddNode adds a node to the graph
func (g *Graph) AddNode(node *Node) error {
	if _, exists := g.Nodes[node.ID]; exists {
		return fmt.Errorf("node already exists: %s", node.ID)
	}

	g.Nodes[node.ID] = node
	return nil
}

// GetNode retrieves a node by ID
func (g *Graph) GetNode(id string) (*Node, bool) {
	node, exists := g.Nodes[id]
	return node, exists
}

// AddDependency adds a dependency between two nodes
func (g *Graph) AddDependency(fromID, toID string) error {
	fromNode, fromExists := g.Nodes[fromID]
	toNode, toExists := g.Nodes[toID]

	if !fromExists {
		return fmt.Errorf("source node not found: %s", fromID)
	}

	if !toExists {
		return fmt.Errorf("target node not found: %s", toID)
	}

	// check dupes
	for _, dep := range fromNode.Dependencies {
		if dep.ID == toID {
			return nil // this dep already exists
		}
	}

	// add the dependency & check for cyclical dependencies
	fromNode.Dependencies = append(fromNode.Dependencies, toNode)
	if g.hasCycle() {
		fromNode.Dependencies = fromNode.Dependencies[:len(fromNode.Dependencies)-1]
		return fmt.Errorf("adding dependency from %s to %s would create a cycle", fromID, toID)
	}

	return nil
}

// hasCycle checks if the graph has any cycles
func (g *Graph) hasCycle() bool {
	visited := make(map[string]bool)
	path := make(map[string]bool)

	for nodeID := range g.Nodes {
		if !visited[nodeID] {
			if g.dfsHasCycle(nodeID, visited, path) {
				return true
			}
		}
	}

	return false
}

// dfsHasCycle is a helper for cycle detection using depth-first search
func (g *Graph) dfsHasCycle(nodeID string, visited, path map[string]bool) bool {
	visited[nodeID] = true
	path[nodeID] = true

	node := g.Nodes[nodeID]
	for _, dep := range node.Dependencies {
		if !visited[dep.ID] {
			if g.dfsHasCycle(dep.ID, visited, path) {
				return true
			}
		} else if path[dep.ID] {
			return true
		}
	}

	path[nodeID] = false
	return false
}

// TopologicalSort returns the nodes in topological order
func (g *Graph) TopologicalSort() ([]*Node, error) {
	if g.hasCycle() {
		return nil, errors.New("graph has cycles, cannot perform topological sort")
	}

	visited := make(map[string]bool)
	var result []*Node
	for _, node := range g.EntryPoints {
		if !visited[node.ID] {
			g.dfs(node.ID, visited, &result)
		}
	}

	for nodeID := range g.Nodes {
		if !visited[nodeID] {
			g.dfs(nodeID, visited, &result)
		}
	}

	// reverse the result to get topological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}

// dfs is a helper for topological sort using depth-first search
func (g *Graph) dfs(nodeID string, visited map[string]bool, result *[]*Node) {
	visited[nodeID] = true

	node := g.Nodes[nodeID]
	for _, dep := range node.Dependencies {
		if !visited[dep.ID] {
			g.dfs(dep.ID, visited, result)
		}
	}

	*result = append(*result, node)
}

// GetBuildOrder returns the nodes in build order
func (g *Graph) GetBuildOrder() ([]*Node, error) {
	return g.TopologicalSort()
}

// MarkEntryPoint marks a node as an entry point
func (g *Graph) MarkEntryPoint(nodeID string) error {
	node, exists := g.Nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	// Check if already an entry point
	for _, ep := range g.EntryPoints {
		if ep.ID == nodeID {
			return nil
		}
	}

	g.EntryPoints = append(g.EntryPoints, node)
	return nil
}

// GetDependents returns all nodes that depend on the given node
func (g *Graph) GetDependents(nodeID string) []*Node {
	var dependents []*Node

	for _, node := range g.Nodes {
		for _, dep := range node.Dependencies {
			if dep.ID == nodeID {
				dependents = append(dependents, node)
				break
			}
		}
	}

	return dependents
}

// GetDependentsRecursive returns all nodes that directly or indirectly depend on the given node
func (g *Graph) GetDependentsRecursive(nodeID string) []*Node {
	visited := make(map[string]bool)
	var result []*Node

	g.getDependentsRecursive(nodeID, visited, &result)

	return result
}

// getDependentsRecursive is a helper for GetDependentsRecursive
func (g *Graph) getDependentsRecursive(nodeID string, visited map[string]bool, result *[]*Node) {
	if visited[nodeID] {
		return
	}

	visited[nodeID] = true
	directDependents := g.GetDependents(nodeID)
	for _, dep := range directDependents {
		*result = append(*result, dep)
		g.getDependentsRecursive(dep.ID, visited, result)
	}
}
