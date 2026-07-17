package proxy_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stelmakhdigital/ai"
	"stell/agent/proxy"
)

func TestStreamProxySSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stream" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"text_delta\",\"delta\":\"hi\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"done\",\"reason\":\"stop\",\"usage\":{\"inputTokens\":1,\"outputTokens\":1}}\n\n")
	}))
	defer srv.Close()

	fn := proxy.StreamProxy(proxy.Options{ProxyURL: srv.URL, AuthToken: "secret", HTTPClient: srv.Client()})
	ch, err := fn(context.Background(), ai.ChatRequest{Model: "m", Messages: []ai.Message{{Role: ai.RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	var tokens strings.Builder
	var done bool
	for ev := range ch {
		switch ev.Type {
		case ai.EventToken:
			tokens.WriteString(ev.Token)
		case ai.EventDone:
			done = true
		case ai.EventError:
			t.Fatalf("error: %v", ev.Err)
		}
	}
	if tokens.String() != "hi" || !done {
		t.Fatalf("tokens=%q done=%v", tokens.String(), done)
	}
}

func TestStreamProxyHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream down"}`))
	}))
	defer srv.Close()

	fn := proxy.StreamProxy(proxy.Options{ProxyURL: srv.URL, AuthToken: "t", HTTPClient: srv.Client()})
	_, err := fn(context.Background(), ai.ChatRequest{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "upstream down") {
		t.Fatalf("err=%v", err)
	}
}
