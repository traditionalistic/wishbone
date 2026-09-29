package ops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	msgAuth     = "Войдите в аккаунт, чтобы участвовать в розыгрыше"
	msgUpstream = "Не удалось загрузить розыгрыш"
)

// GQL talks to the NewBackend GraphQL upstream (spec 2026-09-28 §3.1).
type GQL struct {
	URL   string       // "" → every call fails with UPSTREAM_FAILED
	Login string       // "phone:code" of a stage test account: the identity of reads without Authorization
	HTTP  *http.Client // nil → upstreamClient (pooled, 10 s timeout)
	Now   func() time.Time

	mu    sync.Mutex
	token string
	exp   time.Time
}

// failure is a domain error of an upstream call; handlers turn it into ops.Fail(code, msg).
type failure struct{ code, msg string }

func (f *failure) Error() string { return f.msg }

// Do runs one named operation. auth is the caller's Authorization value or "" (the fallback identity, reads only).
func (g *GQL) Do(ctx context.Context, auth string, write bool, name, query string, vars map[string]any, out any) error {
	fallback := auth == ""
	if fallback {
		if write || g.Login == "" {
			return &failure{"UNAUTHENTICATED", msgAuth}
		}
		t, err := g.fallbackToken(ctx)
		if err != nil {
			return err
		}
		auth = "Bearer " + t
	}
	err := g.post(ctx, auth, name, query, vars, out)
	var f *failure
	if fallback && errors.As(err, &f) && f.code == "UNAUTHENTICATED" {
		g.mu.Lock()
		g.token = "" // expired early or revoked: log in again next time
		g.mu.Unlock()
	}
	return err
}

func (g *GQL) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// upstreamClient is shared by every GQL without its own client: DefaultTransport keeps 2 idle connections per
// host, which under concurrent /v1/data calls means a new TLS handshake to the one upstream host most of the time.
var upstreamClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: func() *http.Transport {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.MaxIdleConnsPerHost = 32
		return t
	}(),
}

func (g *GQL) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return upstreamClient
}

// sink decodes a GraphQL "data" member into out; present is false for a missing or null member.
type sink struct {
	out     any
	present bool
	err     error // out's decode error: reported after UNAUTHENTICATED, as when the body was parsed in two steps
}

func (s *sink) UnmarshalJSON(b []byte) error {
	if string(b) == "null" || s.out == nil {
		s.present = string(b) != "null"
		return nil
	}
	s.present = true
	s.err = json.Unmarshal(b, s.out)
	return nil
}

// post sends one GraphQL request and decodes its data into out. Accept-Language is not forwarded (ru only,
// spec ceiling). Never logs the request/response body.
func (g *GQL) post(ctx context.Context, auth, name, query string, vars map[string]any, out any) error {
	if g.URL == "" {
		return &failure{"UPSTREAM_FAILED", msgUpstream}
	}
	body, err := json.Marshal(map[string]any{"operationName": name, "query": query, "variables": vars})
	if err != nil {
		return &failure{"UPSTREAM_FAILED", msgUpstream}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.URL, bytes.NewReader(body))
	if err != nil {
		return &failure{"UPSTREAM_FAILED", msgUpstream}
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := g.client().Do(req)
	if err != nil {
		return &failure{"UPSTREAM_FAILED", msgUpstream}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) // a drained body keeps the connection reusable
		resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return &failure{"UPSTREAM_FAILED", msgUpstream}
	}

	var wire struct {
		Data   sink `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	wire.Data.out = out // decoded straight from the response stream: no ReadAll copy, no intermediate RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return &failure{"UPSTREAM_FAILED", msgUpstream}
	}
	for _, e := range wire.Errors {
		if e.Extensions.Code == "UNAUTHENTICATED" {
			return &failure{"UNAUTHENTICATED", msgAuth}
		}
	}
	if !wire.Data.present || wire.Data.err != nil {
		return &failure{"UPSTREAM_FAILED", msgUpstream}
	}
	return nil
}

const wishboneLoginQuery = `mutation WishboneLogin($p: String!, $c: Int!) { authenticate(phoneNumber: $p, code: $c) { accessToken } }`

// fallbackToken returns a cached access token for the stage test account (g.Login), logging in again once
// it is within 60s of expiry.
func (g *GQL) fallbackToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.token != "" && g.now().Before(g.exp.Add(-60*time.Second)) {
		return g.token, nil
	}

	phone, codeStr, ok := strings.Cut(g.Login, ":")
	if !ok {
		slog.Error("gql: fallback login is not in phone:code format")
		return "", &failure{"UPSTREAM_FAILED", msgUpstream}
	}
	code, err := strconv.Atoi(codeStr)
	if err != nil {
		slog.Error("gql: fallback login code is not numeric")
		return "", &failure{"UPSTREAM_FAILED", msgUpstream}
	}

	var resp struct {
		Authenticate struct {
			AccessToken string `json:"accessToken"`
		} `json:"authenticate"`
	}
	if err := g.post(ctx, "", "WishboneLogin", wishboneLoginQuery, map[string]any{"p": phone, "c": code}, &resp); err != nil {
		return "", err
	}

	tok := resp.Authenticate.AccessToken
	exp := g.now().Add(5 * time.Minute) // missing/unparsable exp: cache conservatively
	if parts := strings.Split(tok, "."); len(parts) == 3 {
		if payload, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
			var claims struct {
				Exp int64 `json:"exp"`
			}
			if json.Unmarshal(payload, &claims) == nil && claims.Exp > 0 {
				exp = time.Unix(claims.Exp, 0)
			}
		}
	}

	g.token, g.exp = tok, exp
	return tok, nil
}

// fail converts any error of Do into a Result.
func fail(err error) Result {
	var f *failure
	if errors.As(err, &f) {
		return Fail(f.code, f.msg)
	}
	slog.Warn("upstream call failed", "err", err)
	return Fail("UPSTREAM_FAILED", msgUpstream)
}
