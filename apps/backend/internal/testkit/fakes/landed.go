package fakes

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	fakeSlot     = 451000000
	usdcDecimals = 6
	swapPool     = "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	swapPoolATA  = "5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1"
)

type landedSwap struct {
	swapOrder
	signature string
}

func (s *Server) land(signature string, order swapOrder) {
	s.mu.Lock()
	s.landed[signature] = landedSwap{swapOrder: order, signature: signature}
	s.mu.Unlock()
}

func (s *Server) landedSwap(signature string) (landedSwap, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.landed[signature]
	return l, ok
}

func (s *Server) signatureStatuses(w http.ResponseWriter, r *http.Request) {
	var call rpcCall
	var sigs []string
	if json.NewDecoder(r.Body).Decode(&call) != nil || len(call.Params) == 0 ||
		json.Unmarshal(call.Params[0], &sigs) != nil {
		rpcReply(w, call.ID, nil, &rpcFault{Code: -32602, Message: "invalid params"})
		return
	}
	value := make([]any, len(sigs))
	for i, sig := range sigs {
		value[i] = s.signatureStatus(sig)
	}
	rpcReply(w, call.ID, map[string]any{"context": map[string]any{"slot": fakeSlot}, "value": value}, nil)
}

func (s *Server) signatureStatus(sig string) any {
	if status, ok := s.recordedStatus(sig); ok {
		return status
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, landed := s.landed[sig]; !landed && s.unexecuted[sig] || !wellFormed(sig) {
		return nil
	}
	return map[string]any{"slot": fakeSlot, "confirmations": nil, "err": nil, "confirmationStatus": "finalized"}
}

func wellFormed(sig string) bool {
	raw, ok := chain.DecodeBase58(sig)
	return ok && len(raw) == ed25519.SignatureSize
}

func (s *Server) recordedStatus(sig string) (json.RawMessage, bool) {
	f, ok := s.fixtures["/rpc/getSignatureStatuses/"+sig]
	if !ok {
		return nil, false
	}
	var reply struct {
		Result struct {
			Value []json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(f.Body, &reply) != nil || len(reply.Result.Value) != 1 {
		return nil, false
	}
	return reply.Result.Value[0], true
}

func (s *Server) blockhashValid(w http.ResponseWriter, r *http.Request) {
	var call rpcCall
	var hash string
	if json.NewDecoder(r.Body).Decode(&call) != nil || len(call.Params) == 0 ||
		json.Unmarshal(call.Params[0], &hash) != nil {
		rpcReply(w, call.ID, nil, &rpcFault{Code: -32602, Message: "invalid params"})
		return
	}
	s.mu.Lock()
	_, ordered := s.blockhashes[hash]
	s.mu.Unlock()
	rpcReply(w, call.ID, map[string]any{"context": map[string]any{"slot": fakeSlot}, "value": !ordered}, nil)
}

func (s *Server) swapTransaction(w http.ResponseWriter, r *http.Request) {
	var call rpcCall
	var sig string
	if json.NewDecoder(r.Body).Decode(&call) != nil || len(call.Params) == 0 ||
		json.Unmarshal(call.Params[0], &sig) != nil {
		rpcReply(w, call.ID, nil, &rpcFault{Code: -32602, Message: "invalid params"})
		return
	}
	landed, ok := s.landedSwap(sig)
	if !ok {
		s.serveFixture(w, []string{"/rpc/getTransaction/" + sig, "/rpc/getTransaction"})
		return
	}
	rpcReply(w, call.ID, s.inboundTransaction(landed), nil)
}

func (s *Server) inboundTransaction(l landedSwap) map[string]any {
	decimals := s.mintDecimals(l.outMint)
	ata, _ := chain.AssociatedTokenAccount(
		chain.SolanaAddress(l.taker), chain.SolanaAddress(l.outMint), chain.SPLProgram,
	)
	balance := func(index int, owner, amount string) map[string]any {
		return map[string]any{
			"accountIndex": index, "mint": l.outMint, "owner": owner, "programId": chain.SPLProgram,
			"uiTokenAmount": map[string]any{"amount": amount, "decimals": decimals},
		}
	}
	return map[string]any{
		"slot": fakeSlot, "blockTime": 1790000000,
		"meta": map[string]any{
			"err": nil, "fee": 5000,
			"preTokenBalances":  []any{balance(1, swapPool, l.out), balance(2, l.taker, "0")},
			"postTokenBalances": []any{balance(1, swapPool, "0"), balance(2, l.taker, l.out)},
			"innerInstructions": []any{},
		},
		"transaction": map[string]any{
			"signatures": []string{l.signature},
			"message": map[string]any{
				"accountKeys": []any{
					map[string]any{"pubkey": l.taker, "signer": true, "writable": true},
					map[string]any{"pubkey": swapPoolATA, "signer": false, "writable": true},
					map[string]any{"pubkey": string(ata), "signer": false, "writable": true},
					map[string]any{"pubkey": l.outMint, "signer": false, "writable": false},
				},
				"instructions": []any{map[string]any{
					"program": "spl-token", "programId": chain.SPLProgram,
					"parsed": map[string]any{"type": "transferChecked", "info": map[string]any{
						"source": swapPoolATA, "destination": string(ata), "authority": swapPool, "mint": l.outMint,
						"tokenAmount": map[string]any{"amount": l.out, "decimals": decimals},
					}},
				}},
			},
		},
	}
}

func (s *Server) mintDecimals(mint string) int {
	var reply struct {
		Result struct {
			Value struct {
				Data struct {
					Parsed struct {
						Info struct {
							Decimals int `json:"decimals"`
						} `json:"info"`
					} `json:"parsed"`
				} `json:"data"`
			} `json:"value"`
		} `json:"result"`
	}
	f, ok := s.fixtures["/rpc/getAccountInfo/"+mint]
	if !ok || json.Unmarshal(f.Body, &reply) != nil {
		return usdcDecimals
	}
	return reply.Result.Value.Data.Parsed.Info.Decimals
}
