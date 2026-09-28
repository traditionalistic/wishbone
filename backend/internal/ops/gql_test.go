package ops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// upstream answers by operationName with a canned body and records the Authorization it saw.
type upstream struct {
	t      *testing.T
	bodies map[string]string // operationName → response body
	auth   []string
	logins int
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	u.auth = append(u.auth, r.Header.Get("Authorization"))
	if req.OperationName == "WishboneLogin" {
		u.logins++
		exp := time.Now().Add(15 * time.Minute).Unix()
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"id":1188}`, exp)))
		fmt.Fprintf(w, `{"data":{"authenticate":{"accessToken":"h.%s.s"}}}`, payload)
		return
	}
	b, ok := u.bodies[req.OperationName]
	if !ok {
		w.WriteHeader(500)
		return
	}
	io.WriteString(w, b)
}

func TestGQLForwardsAuthorization(t *testing.T) {
	u := &upstream{t: t, bodies: map[string]string{"Q": `{"data":{"x":1}}`}}
	srv := httptest.NewServer(u)
	defer srv.Close()

	g := &GQL{URL: srv.URL}
	var out struct {
		X int `json:"x"`
	}
	if err := g.Do(context.Background(), "Bearer u", false, "Q", "query Q { x }", nil, &out); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if out.X != 1 {
		t.Fatalf("out.X = %d, want 1", out.X)
	}
	if len(u.auth) != 1 || u.auth[0] != "Bearer u" {
		t.Fatalf("auth seen = %v, want [Bearer u]", u.auth)
	}
}

func TestGQLFallbackLoginForReadsOnly(t *testing.T) {
	u := &upstream{t: t, bodies: map[string]string{"Q": `{"data":{"x":1}}`}}
	srv := httptest.NewServer(u)
	defer srv.Close()

	now := time.Now()
	g := &GQL{URL: srv.URL, Login: "79956791664:133767", Now: func() time.Time { return now }}
	var out struct {
		X int `json:"x"`
	}

	for i := 0; i < 2; i++ {
		if err := g.Do(context.Background(), "", false, "Q", "query Q { x }", nil, &out); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}
	if u.logins != 1 {
		t.Fatalf("logins = %d, want 1", u.logins)
	}
	if len(u.auth) != 3 {
		t.Fatalf("auth seen = %v, want 3 entries (1 login + 2 reads)", u.auth)
	}
	for _, a := range u.auth[1:] {
		if a == "" || a[:7] != "Bearer " {
			t.Fatalf("read auth = %q, want a Bearer token", a)
		}
	}
	if u.auth[1] != u.auth[2] {
		t.Fatalf("reads used different tokens: %q vs %q", u.auth[1], u.auth[2])
	}

	err := g.Do(context.Background(), "", true, "Q", "query Q { x }", nil, &out)
	var f *failure
	if !errors.As(err, &f) || f.code != "UNAUTHENTICATED" {
		t.Fatalf("write without auth: err = %v, want *failure{UNAUTHENTICATED}", err)
	}
	if len(u.auth) != 3 {
		t.Fatalf("write without auth must not call upstream: auth = %v", u.auth)
	}

	now = now.Add(15 * time.Minute) // past exp-60s: cached token must be re-fetched
	if err := g.Do(context.Background(), "", false, "Q", "query Q { x }", nil, &out); err != nil {
		t.Fatalf("read after expiry: %v", err)
	}
	if u.logins != 2 {
		t.Fatalf("logins after expiry = %d, want 2", u.logins)
	}
}

func TestGQLErrors(t *testing.T) {
	t.Run("unauthenticated code", func(t *testing.T) {
		u := &upstream{t: t, bodies: map[string]string{
			"Q": `{"errors":[{"extensions":{"code":"UNAUTHENTICATED"}}],"data":null}`,
		}}
		srv := httptest.NewServer(u)
		defer srv.Close()
		g := &GQL{URL: srv.URL}
		var out struct{}
		err := g.Do(context.Background(), "Bearer x", false, "Q", "query Q { x }", nil, &out)
		var f *failure
		if !errors.As(err, &f) || f.code != "UNAUTHENTICATED" {
			t.Fatalf("err = %v, want *failure{UNAUTHENTICATED}", err)
		}
	})

	t.Run("generic graphql error", func(t *testing.T) {
		u := &upstream{t: t, bodies: map[string]string{
			"Q": `{"errors":[{"message":"boom"}],"data":null}`,
		}}
		srv := httptest.NewServer(u)
		defer srv.Close()
		g := &GQL{URL: srv.URL}
		var out struct{}
		err := g.Do(context.Background(), "Bearer x", false, "Q", "query Q { x }", nil, &out)
		var f *failure
		if !errors.As(err, &f) || f.code != "UPSTREAM_FAILED" {
			t.Fatalf("err = %v, want *failure{UPSTREAM_FAILED}", err)
		}
	})

	t.Run("http 500", func(t *testing.T) {
		u := &upstream{t: t, bodies: map[string]string{}}
		srv := httptest.NewServer(u)
		defer srv.Close()
		g := &GQL{URL: srv.URL}
		var out struct{}
		err := g.Do(context.Background(), "Bearer x", false, "Q", "query Q { x }", nil, &out)
		var f *failure
		if !errors.As(err, &f) || f.code != "UPSTREAM_FAILED" {
			t.Fatalf("err = %v, want *failure{UPSTREAM_FAILED}", err)
		}
	})

	t.Run("empty URL", func(t *testing.T) {
		g := &GQL{}
		var out struct{}
		err := g.Do(context.Background(), "Bearer x", false, "Q", "query Q { x }", nil, &out)
		var f *failure
		if !errors.As(err, &f) || f.code != "UPSTREAM_FAILED" {
			t.Fatalf("err = %v, want *failure{UPSTREAM_FAILED}", err)
		}
	})

	t.Run("partial errors with data still decode", func(t *testing.T) {
		u := &upstream{t: t, bodies: map[string]string{
			"Q": `{"errors":[{"message":"partial"}],"data":{"x":1}}`,
		}}
		srv := httptest.NewServer(u)
		defer srv.Close()
		g := &GQL{URL: srv.URL}
		var out struct {
			X int `json:"x"`
		}
		if err := g.Do(context.Background(), "Bearer x", false, "Q", "query Q { x }", nil, &out); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if out.X != 1 {
			t.Fatalf("out.X = %d, want 1", out.X)
		}
	})

	t.Run("unauthenticated clears cached fallback token", func(t *testing.T) {
		u := &upstream{t: t, bodies: map[string]string{
			"Q": `{"errors":[{"extensions":{"code":"UNAUTHENTICATED"}}],"data":null}`,
		}}
		srv := httptest.NewServer(u)
		defer srv.Close()
		g := &GQL{URL: srv.URL, Login: "79956791664:133767"}
		var out struct{}

		_ = g.Do(context.Background(), "", false, "Q", "query Q { x }", nil, &out)
		if u.logins != 1 {
			t.Fatalf("logins = %d, want 1", u.logins)
		}
		_ = g.Do(context.Background(), "", false, "Q", "query Q { x }", nil, &out)
		if u.logins != 2 {
			t.Fatalf("logins after UNAUTHENTICATED = %d, want 2 (cache must be cleared)", u.logins)
		}
	})
}

func TestFailResult(t *testing.T) {
	r := fail(&failure{"UNAUTHENTICATED", "войдите"})
	if r.OK || r.Error == nil || r.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("fail(*failure) = %+v", r)
	}
	r2 := fail(errors.New("x"))
	if r2.OK || r2.Error == nil || r2.Error.Code != "UPSTREAM_FAILED" {
		t.Fatalf("fail(error) = %+v", r2)
	}
}
