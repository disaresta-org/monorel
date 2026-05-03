package bitbucket

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_RejectsMissingEmail(t *testing.T) {
	_, err := New(context.Background(), Options{Workspace: "ws", Repo: "r", Token: "t"})
	if err == nil {
		t.Fatal("expected error for missing email")
	}
}

func TestNew_RejectsMissingToken(t *testing.T) {
	_, err := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com"})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestNew_RejectsMissingWorkspaceOrRepo(t *testing.T) {
	for _, tc := range []Options{
		{Repo: "r", Email: "e@x.com", Token: "t"},
		{Workspace: "ws", Email: "e@x.com", Token: "t"},
	} {
		if _, err := New(context.Background(), tc); err == nil {
			t.Errorf("expected error for %+v", tc)
		}
	}
}

func TestNew_RejectsNonEmptyHost(t *testing.T) {
	_, err := New(context.Background(), Options{
		Workspace: "ws", Repo: "r", Host: "self-hosted",
		Email: "e@x.com", Token: "t",
	})
	if err == nil {
		t.Fatal("expected error for non-empty Host (Cloud-only)")
	}
}

func TestNew_HappyPath(t *testing.T) {
	c, err := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "t",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestClient_AuthHeader(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = srv.URL

	if _, err := c.do(context.Background(), "GET", "/some-path", nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("e@x.com:tok"))
	if seen != want {
		t.Errorf("Authorization header = %q, want %q", seen, want)
	}
}

func TestClient_DoMapsStatus(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string // substring of err message
	}{
		{401, `{"error":{"message":"unauth"}}`, "auth failed"},
		{403, `{"error":{"message":"no scope"}}`, "forbidden"},
		{404, `{"error":{"message":"missing"}}`, "not found"},
		{402, `{"error":{"message":"plan"}}`, "plan not configured"},
		{429, `{"error":{"message":"slow down"}}`, "rate limited"},
		{500, `{"error":{"message":"oops"}}`, "server error"},
		{400, `{"error":{"message":"bad input"}}`, "bad input"},
	}
	for _, tc := range cases {
		t.Run(strings.TrimSpace(tc.want), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c, _ := New(context.Background(), Options{
				Workspace: "ws", Repo: "r",
				Email: "e@x.com", Token: "tok",
			})
			c.baseURL = srv.URL

			_, err := c.do(context.Background(), "GET", "/x", nil, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}
