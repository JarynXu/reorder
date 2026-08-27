package reorder

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

type sourceUnit struct {
	decl  ast.Decl
	start int
	end   int
	fn    *ast.FuncDecl
}

type typeHolder struct {
	typeUnit     int
	constructors []int
	methods      []int
}

// PlanFile parses src and returns a safe, deterministic reorder plan.
// The implemented semantics intentionally follow funcorder's current checks:
// constructor placement, exported-before-unexported methods, optional
// alphabetical ordering for constructors/method groups, and optional
// exported-before-unexported top-level functions. init is excluded from the
// function rule and is never moved as an anchor.
func PlanFile(filename string, src []byte, cfg Config) (Plan, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return Plan{}, fmt.Errorf("parse %s: %w", filename, err)
	}

	units, err := buildSourceUnits(fset, file, src)
	if err != nil {
		return Plan{}, err
	}
	if len(units) < 2 {
		return Plan{}, nil
	}

	order, err := desiredOrder(file, units, cfg)
	if err != nil {
		return Plan{}, fmt.Errorf("plan %s: %w", filename, err)
	}

	first, last := changedRange(order)
	if first < 0 {
		return Plan{}, nil
	}

	for boundary := first; boundary < last; boundary++ {
		gap := src[units[boundary].end:units[boundary+1].start]
		if onlyWhitespace(gap) || !crossesBoundary(order, boundary) {
			continue
		}
		return Plan{}, &UnsafeTriviaError{Filename: filename, Offset: units[boundary].end}
	}

	var replacement bytes.Buffer
	for pos := first; pos <= last; pos++ {
		unit := units[order[pos]]
		replacement.Write(src[unit.start:unit.end])
		if pos < last {
			replacement.Write(src[units[pos].end:units[pos+1].start])
		}
	}

	return Plan{Edit: &Edit{
		Start:   units[first].start,
		End:     units[last].end,
		NewText: replacement.Bytes(),
	}}, nil
}

func buildSourceUnits(fset *token.FileSet, file *ast.File, src []byte) ([]sourceUnit, error) {
	units := make([]sourceUnit, 0, len(file.Decls))
	previousEnd := 0

	for _, decl := range file.Decls {
		start := positionOffset(fset, decl.Pos())
		end := positionOffset(fset, decl.End())

		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil {
				start = positionOffset(fset, d.Doc.Pos())
			}
		case *ast.GenDecl:
			if d.Doc != nil {
				start = positionOffset(fset, d.Doc.Pos())
			}
		}

		end = extendSameLineTrailingComment(fset, file, decl.End(), end)
		if start < previousEnd || start < 0 || end < start || end > len(src) {
			return nil, fmt.Errorf("invalid declaration source span [%d,%d)", start, end)
		}

		unit := sourceUnit{decl: decl, start: start, end: end}
		if fn, ok := decl.(*ast.FuncDecl); ok {
			unit.fn = fn
		}
		units = append(units, unit)
		previousEnd = end
	}

	return units, nil
}

func extendSameLineTrailingComment(fset *token.FileSet, file *ast.File, declEnd token.Pos, end int) int {
	endLine := fset.PositionFor(declEnd, false).Line
	for _, group := range file.Comments {
		if group.Pos() < declEnd {
			continue
		}
		if fset.PositionFor(group.Pos(), false).Line != endLine {
			continue
		}
		if groupEnd := positionOffset(fset, group.End()); groupEnd > end {
			end = groupEnd
		}
	}
	return end
}

func positionOffset(fset *token.FileSet, pos token.Pos) int {
	return fset.PositionFor(pos, false).Offset
}

func desiredOrder(file *ast.File, units []sourceUnit, cfg Config) ([]int, error) {
	graph := newOrderGraph(len(units))

	// Non-function declarations and init functions are anchors. Their relative
	// source order is immutable. Ordinary functions/methods are the only units
	// the planner may move.
	lastAnchor := -1
	for i, unit := range units {
		if !isAnchor(unit) {
			continue
		}
		if lastAnchor >= 0 {
			graph.add(lastAnchor, i)
		}
		lastAnchor = i
	}

	holders := collectTypeHolders(units)

	if cfg.Constructor {
		for _, holder := range holders {
			for _, constructor := range holder.constructors {
				graph.add(holder.typeUnit, constructor)
				for _, method := range holder.methods {
					graph.add(constructor, method)
				}
			}

			if cfg.Alphabetical {
				addAlphabeticalEdges(graph, units, holder.constructors)
			}
		}
	}

	if cfg.StructMethod {
		for _, holder := range holders {
			exported, unexported := splitByExported(units, holder.methods)
			for _, publicMethod := range exported {
				for _, privateMethod := range unexported {
					graph.add(publicMethod, privateMethod)
				}
			}

			if cfg.Alphabetical {
				addAlphabeticalEdges(graph, units, exported)
				addAlphabeticalEdges(graph, units, unexported)
			}
		}
	}

	if cfg.Function {
		var functions []int
		for i, unit := range units {
			if unit.fn == nil || unit.fn.Recv != nil || unit.fn.Name.Name == "init" {
				continue
			}
			functions = append(functions, i)
		}
		exported, unexported := splitByExported(units, functions)
		for _, publicFunc := range exported {
			for _, privateFunc := range unexported {
				graph.add(publicFunc, privateFunc)
			}
		}
	}

	return graph.stableTopologicalOrder()
}

func isAnchor(unit sourceUnit) bool {
	if unit.fn == nil {
		return true
	}
	return unit.fn.Recv == nil && unit.fn.Name.Name == "init"
}

func collectTypeHolders(units []sourceUnit) map[string]*typeHolder {
	holders := make(map[string]*typeHolder)

	for unitIndex, unit := range units {
		gen, ok := unit.decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			holders[typeSpec.Name.Name] = &typeHolder{typeUnit: unitIndex}
		}
	}

	for unitIndex, unit := range units {
		fn := unit.fn
		if fn == nil {
			continue
		}

		if constructorType := constructorReturnType(fn); constructorType != "" {
			if holder := holders[constructorType]; holder != nil {
				holder.constructors = append(holder.constructors, unitIndex)
			}
			continue
		}

		if receiverType := methodReceiverType(fn); receiverType != "" {
			if holder := holders[receiverType]; holder != nil {
				holder.methods = append(holder.methods, unitIndex)
			}
		}
	}

	return holders
}

func constructorReturnType(fn *ast.FuncDecl) string {
	if !funcCanBeConstructor(fn) {
		return ""
	}
	return identName(fn.Type.Results.List[0].Type)
}

func funcCanBeConstructor(fn *ast.FuncDecl) bool {
	if !fn.Name.IsExported() || fn.Recv != nil {
		return false
	}
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return false
	}

	lower := strings.ToLower(fn.Name.Name)
	for _, prefix := range []string{"new", "must"} {
		if strings.HasPrefix(lower, prefix) && len(fn.Name.Name) > len(prefix) {
			return true
		}
	}
	return false
}

func methodReceiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	return identName(fn.Recv.List[0].Type)
}

func identName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.StarExpr:
		return identName(value.X)
	case *ast.Ident:
		return value.Name
	default:
		return ""
	}
}

func splitByExported(units []sourceUnit, ids []int) (exported, unexported []int) {
	for _, id := range ids {
		if units[id].fn.Name.IsExported() {
			exported = append(exported, id)
		} else {
			unexported = append(unexported, id)
		}
	}
	return exported, unexported
}

func addAlphabeticalEdges(graph *orderGraph, units []sourceUnit, ids []int) {
	ordered := append([]int(nil), ids...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return units[ordered[i]].fn.Name.Name < units[ordered[j]].fn.Name.Name
	})
	for i := 0; i+1 < len(ordered); i++ {
		graph.add(ordered[i], ordered[i+1])
	}
}

func changedRange(order []int) (int, int) {
	first, last := -1, -1
	for i, id := range order {
		if id == i {
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	return first, last
}

func crossesBoundary(order []int, boundary int) bool {
	for pos := 0; pos <= boundary; pos++ {
		if order[pos] > boundary {
			return true
		}
	}
	return false
}

func onlyWhitespace(src []byte) bool {
	return len(bytes.TrimSpace(src)) == 0
}

type orderGraph struct {
	edges    []map[int]struct{}
	indegree []int
}

func newOrderGraph(size int) *orderGraph {
	edges := make([]map[int]struct{}, size)
	for i := range edges {
		edges[i] = make(map[int]struct{})
	}
	return &orderGraph{edges: edges, indegree: make([]int, size)}
}

func (g *orderGraph) add(before, after int) {
	if before == after {
		return
	}
	if _, exists := g.edges[before][after]; exists {
		return
	}
	g.edges[before][after] = struct{}{}
	g.indegree[after]++
}

func (g *orderGraph) stableTopologicalOrder() ([]int, error) {
	indegree := append([]int(nil), g.indegree...)
	used := make([]bool, len(indegree))
	order := make([]int, 0, len(indegree))

	for len(order) < len(indegree) {
		next := -1
		for i := range indegree {
			if !used[i] && indegree[i] == 0 {
				next = i
				break
			}
		}
		if next < 0 {
			return nil, fmt.Errorf("ordering constraints contain a cycle")
		}

		used[next] = true
		order = append(order, next)
		for after := range g.edges[next] {
			indegree[after]--
		}
	}
	return order, nil
}
