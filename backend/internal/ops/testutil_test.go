package ops

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// upstream is a fake GraphQL server: it answers by operationName with a canned body and records the
// Authorization, operation names and variables it saw.
type upstream struct {
	t      *testing.T
	bodies map[string]string // operationName → response body

	mu     sync.Mutex
	auth   []string
	ops    []string
	vars   map[string]map[string]any // operationName → variables of its last request
	logins int
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	u.mu.Lock()
	u.auth = append(u.auth, r.Header.Get("Authorization"))
	u.ops = append(u.ops, req.OperationName)
	if u.vars == nil {
		u.vars = map[string]map[string]any{}
	}
	u.vars[req.OperationName] = req.Variables
	if req.OperationName == "WishboneLogin" {
		u.logins++
	}
	u.mu.Unlock()
	if req.OperationName == "WishboneLogin" {
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
