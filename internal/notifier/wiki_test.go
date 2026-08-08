// Package notifier tests the weekly-to-wiki internal request contract.
package notifier

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWikiNotifierSendBatchSetsAggregateServiceHeader(t *testing.T) {
	var gotService string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotService = r.Header.Get("X-SC-Svc")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := &WikiNotifier{
		apiURL: server.URL,
		apiKey: "test-key",
		client: server.Client(),
	}
	if err := notifier.sendBatch([]string{"starcat-app/starcat"}); err != nil {
		t.Fatalf("sendBatch() error = %v", err)
	}
	if gotService != "wiki" {
		t.Fatalf("X-SC-Svc = %q, want wiki", gotService)
	}
}
