package feed

import (
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func RenderTitle(k Kind, p Payload) string {
	switch k {
	case KindProposal:
		return proposalTitle(p)
	case KindTrade:
		return tradeTitle(p)
	case KindPriceMove:
		return priceMoveTitle(p)
	case KindCabalCreated:
		if p.ActorName == "" {
			return p.CabalName + " is a new cabal"
		}
		return p.ActorName + " started " + p.CabalName
	case KindMemberJoined:
		if p.ActorName == "" {
			return "A new member joined " + p.CabalName
		}
		return p.ActorName + " joined " + p.CabalName
	case KindReply:
		return replyTitle(p)
	case KindThreshold:
		return thresholdTitle(p)
	case KindResult:
		return resultTitle(p)
	default:
		return ""
	}
}

func RenderDetail(k Kind, p Payload) string {
	switch k {
	case KindProposal:
		return statusLabel(p.Status, p.StatusCode)
	case KindTrade:
		if p.PriceMicros.IsZero() {
			return ""
		}
		return "Filled at " + priceOf(p.PriceMicros)
	case KindPriceMove:
		return priceMoveDetail(p)
	case KindReply:
		return join(" · ", p.Symbol, "Replied to the proposal")
	case KindThreshold:
		return "Execution pending"
	case KindResult:
		return resultDetail(p)
	case KindCabalCreated, KindMemberJoined:
		return ""
	default:
		return ""
	}
}

func RenderTone(k Kind, p Payload) Tone {
	switch {
	case k != KindPriceMove || p.ChangeBps == 0:
		return ToneNeutral
	case p.ChangeBps > 0:
		return TonePositive
	default:
		return ToneNegative
	}
}

func proposalTitle(p Payload) string {
	return memberOr(p.ActorName) + " proposed " + tradeVerb(p) + " " + exactAmountOf(p) + " in " + p.CabalName
}

func replyTitle(p Payload) string {
	return memberOr(p.ActorName) + " replied to " + memberOr(p.ProposerName) + "'s proposal"
}

func thresholdTitle(p Payload) string {
	return memberOr(p.ActorName) + "'s proposal to " + tradeVerbBare(p) + " " + exactAmountOf(p) + " was approved"
}

func resultTitle(p Payload) string {
	if p.Shape == ShapeMember {
		return memberResultTitle(p)
	}
	verb := "passed"
	if p.Outcome == OutcomeFailed {
		verb = "failed"
	}
	return join(" ", p.CabalName, verb, capitalized(tradeVerbBare(p)), assetLabel(p), resultDollars(p), tallyOf(p))
}

func memberResultTitle(p Payload) string {
	noun, tally := "buy", ""
	if p.Action == ActionSell {
		noun, tally = "sale", tallyOf(p)
	}
	return join(" ", memberOr(p.ActorName)+"'s", noun, "of", resultAmountOf(p), "in", p.CabalName, "passed", tally)
}

func resultDetail(p Payload) string {
	switch {
	case p.Outcome == OutcomeFailed:
		return "no trade"
	case p.Action == ActionSell:
		return "selling"
	default:
		return "buying"
	}
}

func resultDollars(p Payload) string {
	if p.USDCMicros.IsZero() {
		return ""
	}
	return dollars(p.USDCMicros)
}

func resultAmountOf(p Payload) string {
	return join(" of ", resultDollars(p), assetLabel(p))
}

func tallyOf(p Payload) string {
	if p.Voters == 0 {
		return ""
	}
	return strconv.Itoa(p.Yes) + "–" + strconv.Itoa(p.No)
}

func capitalized(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

func memberOr(name string) string {
	if name == "" {
		return "A member"
	}
	return name
}

func tradeVerb(p Payload) string {
	if p.Action == ActionSell {
		return "selling"
	}
	return "buying"
}

func tradeVerbBare(p Payload) string {
	if p.Action == ActionSell {
		return "sell"
	}
	return "buy"
}

func tradeTitle(p Payload) string {
	verb := "bought"
	if p.Action == ActionSell {
		verb = "sold"
	}
	return p.CabalName + " " + verb + " " + amountOf(p)
}

func assetLabel(p Payload) string {
	if p.AssetName != "" {
		return p.AssetName
	}
	return p.Symbol
}

func amountOf(p Payload) string {
	if p.USDCMicros.IsZero() {
		return assetLabel(p)
	}
	return dollars(p.USDCMicros) + " of " + assetLabel(p)
}

func exactAmountOf(p Payload) string {
	if p.USDCMicros.IsZero() {
		return assetLabel(p)
	}
	return priceOf(p.USDCMicros) + " of " + assetLabel(p)
}

func priceMoveTitle(p Payload) string {
	switch {
	case p.ChangeBps > 0:
		return assetLabel(p) + " is up " + percent(p.ChangeBps) + " today"
	case p.ChangeBps < 0:
		return assetLabel(p) + " is down " + percent(-p.ChangeBps) + " today"
	default:
		return assetLabel(p) + " is flat today"
	}
}

const microsPerCent = 10_000

func dollars(m money.Micros) string {
	cents := m.Uint64() / microsPerCent
	if cents%100 == 0 {
		return "$" + grouped(cents/100)
	}
	return priceOf(m)
}

func priceOf(m money.Micros) string {
	if m.IsZero() {
		return ""
	}
	cents := m.Uint64() / microsPerCent
	return "$" + grouped(cents/100) + "." + strconv.FormatUint(cents%100+100, 10)[1:]
}

func priceMoveDetail(p Payload) string {
	if p.PrevClose.IsZero() {
		return priceOf(p.MarkMicros)
	}
	return join(", ", priceOf(p.MarkMicros), "previous close "+priceOf(p.PrevClose))
}

func percent(bps int64) string {
	if bps%100 == 0 {
		return strconv.FormatInt(bps/100, 10) + "%"
	}
	return strconv.FormatInt(bps/100, 10) + "." + strconv.FormatInt(bps%100+100, 10)[1:] + "%"
}

func grouped(n uint64) string {
	digits := strconv.FormatUint(n, 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}

func join(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}
