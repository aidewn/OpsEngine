package runtime

import (
	"errors"
	"testing"

	"OpsEngine/internal/agent/workflow"
	"OpsEngine/internal/clients"
)

func TestIsExecutionFailureReport(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"01:38:23 脚本执行失败 exit_code=1\nstderr: sed error", true},
		{"执行失败了", true},
		{"请把集合改简洁一点", false},
		{"command failed with exit code 2", true},
	}
	for _, tc := range cases {
		if got := isExecutionFailureReport(tc.msg); got != tc.want {
			t.Fatalf("isExecutionFailureReport(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

// seqLLM 按顺序返回预设回复，用于重试路径单测。
type seqLLM struct {
	replies []string
	idx     int
}

func (s *seqLLM) Chat(_ []clients.ChatMessage) (string, error) {
	if s.idx >= len(s.replies) {
		return "", errors.New("no more replies")
	}
	r := s.replies[s.idx]
	s.idx++
	return r, nil
}

func (s *seqLLM) ChatStream(_ []clients.ChatMessage, _ func(string)) (string, error) {
	return "", errors.New("not implemented")
}

func (s *seqLLM) ChatWithTools(_ []clients.ChatMessage, _ []clients.ToolSpec) (clients.ChatCompletion, error) {
	return clients.ChatCompletion{}, errors.New("not implemented")
}

func (s *seqLLM) ChatWithToolsStream(_ []clients.ChatMessage, _ []clients.ToolSpec, _ func(string)) (clients.ChatCompletion, error) {
	return clients.ChatCompletion{}, errors.New("not implemented")
}

func TestRequestArtifactDraftRetriesOnParseError(t *testing.T) {
	rt := &Runtime{
		LLM: &seqLLM{replies: []string{
			"not json",
			`{"name":"ok","nodes":[{"id":"n1","type_id":"assemble_start","config":{},"position":{"x":0,"y":0}},{"id":"n2","type_id":"assemble_end","config":{},"position":{"x":0,"y":0}}],"edges":[{"from":{"node":"n1","port":"exec_out"},"to":{"node":"n2","port":"exec_in"}}]}`,
		}},
		Emit: &bufEmitter{},
	}
	progress := []string{}
	_, err := rt.requestArtifactDraft(
		Request{RequestID: "r1", SessionID: "s1"},
		"s1",
		&progress,
		"system",
		"user",
		func(d workflow.Draft) error {
			_, err := workflow.MaterializeAssemble(d, "", nil)
			return err
		},
	)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if len(progress) < 3 {
		t.Fatalf("expected progress steps for retry, got %v", progress)
	}
}
