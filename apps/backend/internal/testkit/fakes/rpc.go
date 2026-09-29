package fakes

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type rpcCall struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func routeOf(upstream string, r *http.Request) (string, []string) {
	route := "/" + upstream + r.URL.Path
	if upstream != "rpc" || r.Method != http.MethodPost || r.URL.Path != "/" {
		return route, []string{route}
	}
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	var c rpcCall
	if json.Unmarshal(body, &c) != nil || c.Method == "" {
		return route, []string{route}
	}
	route = "/rpc/" + c.Method
	var first string
	if len(c.Params) > 0 && json.Unmarshal(c.Params[0], &first) == nil && first != "" {
		return route, []string{route + "/" + first, route}
	}
	return route, []string{route}
}

func sendTransaction(w http.ResponseWriter, r *http.Request) {
	var c rpcCall
	var encoded string
	err := json.NewDecoder(r.Body).Decode(&c)
	if err == nil && len(c.Params) > 0 {
		err = json.Unmarshal(c.Params[0], &encoded)
	}
	tx, _ := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(tx) < 1+ed25519.SignatureSize || tx[0] == 0 {
		rpcReply(w, c.ID, nil, &rpcFault{Code: -32602, Message: "invalid transaction"})
		return
	}
	rpcReply(w, c.ID, chain.SignatureOf(tx[1:1+ed25519.SignatureSize]), nil)
}

type rpcFault struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcReply(w http.ResponseWriter, id json.RawMessage, result any, fault *rpcFault) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result,omitempty"`
		Error   *rpcFault       `json:"error,omitempty"`
	}{"2.0", id, result, fault})
}
