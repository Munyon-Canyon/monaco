package fakes

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"net/http"
	"slices"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	orderRoute   = "/jupiter/swap/v2/order"
	executeRoute = "/jupiter/swap/v2/execute"
)

type SwapStatus string

const (
	SwapSuccess SwapStatus = "Success"
	SwapFailed  SwapStatus = "Failed"
	SwapPending SwapStatus = "Pending"
)

type SetSwap struct {
	Owner  string     `json:"owner"`
	Status SwapStatus `json:"status"`
	Code   int        `json:"code,omitempty"`
}

type SetWallet struct {
	ID string `json:"id"`
}

type swapOrder struct {
	taker   string
	in, out string
}

type orderReply struct {
	RequestID      string           `json:"requestId"`
	InputMint      string           `json:"inputMint"`
	OutputMint     string           `json:"outputMint"`
	InAmount       string           `json:"inAmount"`
	OutAmount      string           `json:"outAmount"`
	Router         string           `json:"router"`
	PriceImpactPct string           `json:"priceImpactPct"`
	RoutePlan      []map[string]any `json:"routePlan"`
	Transaction    string           `json:"transaction"`
}

func (s *Server) jupiterOrder(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	taker := q.Get("taker")
	quoted, ok := s.quotedOrder()
	if !ok || !s.holdsWallet(taker) {
		s.serveFixture(w, []string{orderRoute})
		return
	}
	in, okIn := new(big.Int).SetString(q.Get("amount"), 10)
	signers, err := orderSigners(q.Get("payer"), taker)
	if !okIn || in.Sign() <= 0 || err != nil {
		writeStatusJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid order", "errorCode": 400})
		return
	}
	out := scaled(in, quoted)
	s.mu.Lock()
	s.orderCount++
	n := s.orderCount
	id := "req-" + strconv.FormatUint(n, 10)
	s.orders[id] = swapOrder{taker: taker, in: in.String(), out: out}
	s.mu.Unlock()
	writeJSON(w, orderReply{
		RequestID: id, InputMint: q.Get("inputMint"), OutputMint: q.Get("outputMint"),
		InAmount: in.String(), OutAmount: out, Router: "iris", PriceImpactPct: "0.12",
		RoutePlan:   []map[string]any{{"swapInfo": map[string]string{"label": "Meteora DLMM"}, "percent": 100}},
		Transaction: base64.StdEncoding.EncodeToString(unsignedTx(signers, n)),
	})
}

func (s *Server) quotedOrder() (orderReply, bool) {
	var quoted orderReply
	f, ok := s.fixtures[orderRoute]
	return quoted, ok && json.Unmarshal(f.Body, &quoted) == nil && quoted.InAmount != ""
}

func (s *Server) holdsWallet(address string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.wallets {
		if address != "" && w.Address == address {
			return true
		}
	}
	return false
}

func scaled(in *big.Int, quoted orderReply) string {
	num, okNum := new(big.Int).SetString(quoted.OutAmount, 10)
	den, okDen := new(big.Int).SetString(quoted.InAmount, 10)
	if !okNum || !okDen || den.Sign() == 0 {
		return "0"
	}
	return new(big.Int).Quo(new(big.Int).Mul(in, num), den).String()
}

func orderSigners(payer, taker string) ([]chain.SolanaAddress, error) {
	signers := []string{taker}
	if payer != "" && payer != taker {
		signers = []string{payer, taker}
	}
	out := make([]chain.SolanaAddress, len(signers))
	for i, s := range signers {
		addr, err := chain.ParseAddress(s)
		if err != nil {
			return nil, err
		}
		out[i] = addr
	}
	return out, nil
}

func unsignedTx(signers []chain.SolanaAddress, nonce uint64) []byte {
	n := len(signers)
	blockhash := make([]byte, ed25519.PublicKeySize)
	binary.BigEndian.PutUint64(blockhash[len(blockhash)-8:], nonce)
	required := byte(1)
	if n == 2 {
		required = 2
	}
	msg := append([]byte{required, 0, 0}, chain.CompactU16(n)...)
	for _, s := range signers {
		key, _ := s.Bytes()
		msg = append(msg, key...)
	}
	msg = append(append(msg, blockhash...), 0)
	return append(append(chain.CompactU16(n), make([]byte, n*ed25519.SignatureSize)...), msg...)
}

func fullySignedBy(tx chain.Transaction, taker string) bool {
	for i := range tx.Signers {
		if !tx.Signed(i) {
			return false
		}
	}
	return slices.Contains(tx.Signers, chain.SolanaAddress(taker))
}

func (s *Server) jupiterExecute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SignedTransaction string `json:"signedTransaction"`
		RequestID         string `json:"requestId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	s.mu.Lock()
	order, known := s.orders[req.RequestID]
	outcome := s.swaps[order.taker]
	s.mu.Unlock()
	if !known {
		s.serveFixture(w, []string{executeRoute})
		return
	}
	raw, _ := base64.StdEncoding.DecodeString(req.SignedTransaction)
	tx, err := chain.DecodeTransaction(raw)
	if err != nil || !fullySignedBy(tx, order.taker) {
		writeJSON(w, map[string]any{"status": string(SwapFailed), "code": -2, "error": "invalid signed transaction"})
		return
	}
	signature := string(chain.SignatureOf(tx.Signatures[0]))
	switch outcome.Status {
	case SwapFailed:
		writeJSON(w, map[string]any{
			"status": string(SwapFailed), "signature": signature, "code": outcome.Code, "error": "swap failed",
		})
	case SwapPending:
		writeJSON(w, map[string]any{"status": string(SwapPending), "signature": signature, "code": 0})
	case SwapSuccess, "":
		writeJSON(w, map[string]any{
			"status": string(SwapSuccess), "signature": signature, "code": 0,
			"inputAmountResult": order.in, "outputAmountResult": order.out,
		})
	}
}

func (s *Server) setSwap(w http.ResponseWriter, r *http.Request) {
	var sw SetSwap
	if !decodeControl(w, r, &sw) {
		return
	}
	if _, err := chain.ParseAddress(sw.Owner); err != nil {
		http.Error(w, fieldError("owner").Error(), http.StatusBadRequest)
		return
	}
	switch sw.Status {
	case SwapSuccess, SwapFailed, SwapPending:
	default:
		http.Error(w, fieldError("status").Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.swaps[sw.Owner] = sw
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setWallet(w http.ResponseWriter, r *http.Request) {
	var sw SetWallet
	if !decodeControl(w, r, &sw) {
		return
	}
	if sw.ID == "" {
		http.Error(w, fieldError("id").Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.wallets[sw.ID] = privyWallet{
		ID: sw.ID, ChainType: "solana", Address: string(PrivyWalletAddress(sw.ID)),
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func PrivyWalletAddress(walletID string) chain.SolanaAddress {
	return chain.AddressOf(PrivyWalletKey(walletID).Public().(ed25519.PublicKey))
}

func decodeControl(w http.ResponseWriter, r *http.Request, into any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		http.Error(w, "decode: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeStatusJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
