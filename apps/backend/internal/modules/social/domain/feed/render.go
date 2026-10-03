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
	default:
		return ""
	}
}

func RenderDetail(k Kind, p Payload) string {
	switch k {
	case KindProposal:
		return tally(p)
	case KindTrade:
		return p.AssetName
	case KindPriceMove:
		return join(" · ", p.AssetName, priceOf(p.PriceMicros))
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
	verb := "buying"
	if p.Action == ActionSell {
		verb = "selling"
	}
	return p.CabalName + " proposed " + verb + " " + amountOf(p)
}

func tradeTitle(p Payload) string {
	verb := "bought"
	if p.Action == ActionSell {
		verb = "sold"
	}
	return p.CabalName + " " + verb + " " + amountOf(p)
}

func amountOf(p Payload) string {
	if p.USDCMicros.IsZero() {
		return p.Symbol
	}
	return dollars(p.USDCMicros) + " of " + p.Symbol
}

func priceMoveTitle(p Payload) string {
	switch {
	case p.ChangeBps > 0:
		return p.Symbol + " is up " + percent(p.ChangeBps) + " today"
	case p.ChangeBps < 0:
		return p.Symbol + " is down " + percent(-p.ChangeBps) + " today"
	default:
		return p.Symbol + " is flat today"
	}
}

func tally(p Payload) string {
	voters := plural(p.VoterCount, "voter")
	if p.YesVotes+p.NoVotes == 0 {
		return voters
	}
	return strconv.Itoa(p.YesVotes) + " yes, " + strconv.Itoa(p.NoVotes) + " no of " + voters
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
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

func percent(bps int64) string {
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
