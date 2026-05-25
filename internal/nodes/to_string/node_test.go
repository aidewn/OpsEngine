package to_string_test

import (
	"testing"

	"OpsEngine/internal/core"
	"OpsEngine/internal/nodes/to_string"
)

func TestTypeDef(t *testing.T) {
	def := (to_string.Node{}).TypeDef()
	if def.TypeID != "to_string" {
		t.Fatalf("TypeID = %s", def.TypeID)
	}
	if def.NodeKind != core.NodeKindPure {
		t.Fatalf("NodeKind = %s", def.NodeKind)
	}
	if len(def.InputPorts) != 1 || def.InputPorts[0].PortType != core.PortTypeAny {
		t.Fatalf("input port = %+v", def.InputPorts)
	}
	if len(def.OutputPorts) != 1 || def.OutputPorts[0].PortType != core.PortTypeString {
		t.Fatalf("output port = %+v", def.OutputPorts)
	}
}
