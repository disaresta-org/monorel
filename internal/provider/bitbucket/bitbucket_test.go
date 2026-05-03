package bitbucket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"monorel.disaresta.com/internal/provider"
)

func TestNew_RejectsMissingEmail(t *testing.T) {
	_, err := New(context.Background(), Options{Workspace: "ws", Repo: "r", Token: "t"})
	if !errors.Is(err, ErrMissingEmail) {
		t.Fatalf("err = %v, want ErrMissingEmail", err)
	}
}

func TestNew_RejectsMissingToken(t *testing.T) {
	_, err := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com"})
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("err = %v, want ErrMissingToken", err)
	}
}

func TestNew_RejectsMissingWorkspaceOrRepo(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{"missing-workspace", Options{Repo: "r", Email: "e@x.com", Token: "t"}},
		{"missing-repo", Options{Workspace: "ws", Email: "e@x.com", Token: "t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(context.Background(), tc.opts)
			if !errors.Is(err, ErrMissingWorkspaceRepo) {
				t.Errorf("err = %v, want ErrMissingWorkspaceRepo", err)
			}
		})
	}
}

func TestNew_RejectsNonEmptyHost(t *testing.T) {
	_, err := New(context.Background(), Options{
		Workspace: "ws", Repo: "r", Host: "self-hosted",
		Email: "e@x.com", Token: "t",
	})
	if !errors.Is(err, ErrHostNotSupported) {
		t.Fatalf("err = %v, want ErrHostNotSupported", err)
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
	bc := c.(*client)
	bc.baseURL = srv.URL

	if _, err := bc.do(context.Background(), "GET", "/some-path", nil, nil); err != nil {
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
			bc := c.(*client)
			bc.baseURL = srv.URL

			_, err := bc.do(context.Background(), "GET", "/x", nil, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestIdentityProbe_CachesUsername(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			calls++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"username":"theo-bb","display_name":"Theo","uuid":"{abc}"}`)
			return
		}
		t.Errorf("unexpected path %s", r.URL.Path)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "tok",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := bc.resolveUsername(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "theo-bb" {
		t.Errorf("got %q, want theo-bb", got)
	}

	// Call again; should hit the cache, not the server.
	if _, err := bc.resolveUsername(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected 1 probe call, got %d", calls)
	}
}

func TestIdentityProbe_SurfacesAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		fmt.Fprintln(w, `{"error":{"message":"bad creds"}}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r",
		Email: "e@x.com", Token: "tok",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	_, err := bc.resolveUsername(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "auth failed") {
		t.Errorf("err = %v", err)
	}
}

func TestGetDefaultBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/ws/r" {
			t.Errorf("path = %s, want /repositories/ws/r", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"mainbranch":{"name":"master","type":"branch"}}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.GetDefaultBranch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "master" {
		t.Errorf("got %q, want master", got)
	}
}

func TestGetDefaultBranch_EmptyMainbranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{
		Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t",
	})
	bc := c.(*client)
	bc.baseURL = srv.URL

	_, err := c.GetDefaultBranch(context.Background())
	if err == nil {
		t.Fatal("expected error for missing mainbranch")
	}
}

func TestFindOpenReleasePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("q")
		want := `state="OPEN" AND source.branch.name="monorel/release"`
		if got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[{
			"id": 7,
			"state": "OPEN",
			"title": "release",
			"summary":{"raw":"body"},
			"source":{"branch":{"name":"monorel/release"}},
			"merge_commit":null,
			"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/7"}}
		}]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindOpenReleasePR(context.Background(), "monorel/release")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Number != 7 || got.State != "open" || got.MergedSHA != "" {
		t.Errorf("got %+v", got)
	}
}

func TestFindOpenReleasePR_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindOpenReleasePR(context.Background(), "monorel/release")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestCreatePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var body struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Source      struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
			} `json:"source"`
			Destination struct {
				Branch struct {
					Name string `json:"name"`
				} `json:"branch"`
			} `json:"destination"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Title != "release" || body.Source.Branch.Name != "monorel/release" || body.Destination.Branch.Name != "main" {
			t.Errorf("body = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"id": 7, "state":"OPEN", "title":"release", "summary":{"raw":"body"},
			"source":{"branch":{"name":"monorel/release"}},
			"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/7"}}
		}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.CreatePR(context.Background(), provider.CreatePROptions{
		Title:      "release",
		Body:       "body",
		HeadBranch: "monorel/release",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 7 {
		t.Errorf("got #%d, want #7", got.Number)
	}
}

func TestUpdatePR(t *testing.T) {
	cases := []struct {
		name      string
		opts      provider.UpdatePROptions
		wantBody  bool
		wantTitle bool
	}{
		{"both", makeUpdateOpts("new title", "new description"), true, true},
		{"title-only", makeUpdateOptsTitleOnly("new title"), false, true},
		{"body-only", makeUpdateOptsBodyOnly("new description"), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PUT" {
					t.Errorf("method = %s, want PUT", r.Method)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if tc.wantTitle {
					if _, ok := body["title"]; !ok {
						t.Error("expected title in patch")
					}
				} else if _, ok := body["title"]; ok {
					t.Error("unexpected title in patch")
				}
				if tc.wantBody {
					if _, ok := body["description"]; !ok {
						t.Error("expected description in patch (Bitbucket field name)")
					}
					if _, ok := body["body"]; ok {
						t.Error("patch should use description, not body")
					}
				} else if _, ok := body["description"]; ok {
					t.Error("unexpected description in patch")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintln(w, `{
					"id": 7, "state":"OPEN", "title":"x", "summary":{"raw":"x"},
					"source":{"branch":{"name":"monorel/release"}},
					"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/7"}}
				}`)
			}))
			defer srv.Close()

			c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
			bc := c.(*client)
			bc.baseURL = srv.URL
			if _, err := c.UpdatePR(context.Background(), 7, tc.opts); err != nil {
				t.Fatalf("UpdatePR: %v", err)
			}
		})
	}
}

func makeUpdateOpts(title, body string) provider.UpdatePROptions {
	return provider.UpdatePROptions{Title: &title, Body: &body}
}
func makeUpdateOptsTitleOnly(title string) provider.UpdatePROptions {
	return provider.UpdatePROptions{Title: &title}
}
func makeUpdateOptsBodyOnly(body string) provider.UpdatePROptions {
	return provider.UpdatePROptions{Body: &body}
}

func TestUpdatePR_NothingToChange(t *testing.T) {
	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	_, err := c.UpdatePR(context.Background(), 7, provider.UpdatePROptions{})
	if err == nil {
		t.Fatal("expected error when both Title and Body are nil")
	}
}

func TestClosePR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repositories/ws/r/pullrequests/7/decline" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(200)
		fmt.Fprintln(w, `{"id":7,"state":"DECLINED"}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	if err := c.ClosePR(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRelease_NoOp(t *testing.T) {
	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	rel, err := c.CreateRelease(context.Background(), provider.CreateReleaseOptions{
		Tag:  "transports/foo/v1.7.0",
		Name: "transports/foo v1.7.0",
		Body: "release body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil {
		t.Fatal("expected non-nil release")
	}
	if rel.Tag != "transports/foo/v1.7.0" {
		t.Errorf("Tag = %q", rel.Tag)
	}
	if !strings.Contains(rel.HTMLURL, "/src/transports/foo/v1.7.0") {
		t.Errorf("HTMLURL = %q; want a /src/<tag> URL", rel.HTMLURL)
	}
}

func TestFindPRByMergeCommit_Bitbucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("q")
		want := `state="MERGED" AND merge_commit.hash="abc123"`
		if got != want {
			t.Errorf("q = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[{
			"id": 42, "state":"MERGED", "title":"release", "summary":{"raw":"body"},
			"source":{"branch":{"name":"monorel/release"}},
			"merge_commit":{"hash":"abc123"},
			"links":{"html":{"href":"https://bitbucket.org/ws/r/pull-requests/42"}}
		}]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindPRByMergeCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Number != 42 || got.State != "closed" || got.MergedSHA != "abc123" {
		t.Errorf("got %+v", got)
	}
}

func TestFindPRByMergeCommit_Bitbucket_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"values":[]}`)
	}))
	defer srv.Close()

	c, _ := New(context.Background(), Options{Workspace: "ws", Repo: "r", Email: "e@x.com", Token: "t"})
	bc := c.(*client)
	bc.baseURL = srv.URL

	got, err := c.FindPRByMergeCommit(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}
