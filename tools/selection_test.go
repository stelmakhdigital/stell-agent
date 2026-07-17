package tools

import (
	"reflect"
	"testing"
)

func TestResolveActiveToolsDefaultCoreOnly(t *testing.T) {
	reg := append(append([]string{}, CoreTools...), CodingTools...)
	active, restrict := ResolveActiveTools(ToolSelection{}, reg)
	if !restrict {
		t.Fatal("expected restrict")
	}
	if !reflect.DeepEqual(active, CoreTools) {
		t.Fatalf("got %v want %v", active, CoreTools)
	}
}

func TestResolveActiveToolsIncludeCoding(t *testing.T) {
	reg := append(append([]string{}, CoreTools...), CodingTools...)
	active, _ := ResolveActiveTools(ToolSelection{IncludeCoding: true}, reg)
	want := append(append([]string{}, CoreTools...), CodingTools...)
	if !reflect.DeepEqual(active, want) {
		t.Fatalf("got %v want %v", active, want)
	}
}

func TestResolveActiveToolsExclude(t *testing.T) {
	active, _ := ResolveActiveTools(ToolSelection{Exclude: []string{"bash"}}, CoreTools)
	if len(active) != 3 || active[0] != "read" {
		t.Fatalf("got %v", active)
	}
}

func TestResolveActiveToolsNoTools(t *testing.T) {
	active, restrict := ResolveActiveTools(ToolSelection{NoTools: true}, CoreTools)
	if !restrict || len(active) != 0 {
		t.Fatalf("got %v restrict=%v", active, restrict)
	}
}
