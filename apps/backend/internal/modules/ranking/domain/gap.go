package domain

const (
	maxCallsToPass = 50
	bpsPerPercent  = 100
)

type GapVersus string

const (
	VersusNone  GapVersus = ""
	VersusAbove GapVersus = "above"
	VersusBelow GapVersus = "below"
)

type Gap struct {
	Versus         GapVersus
	Points         int
	CallsToPass    *int
	AboveIsPerfect bool
}

func RatePct(rateBps int) int { return (rateBps + bpsPerPercent/2) / bpsPerPercent }

func CallsToPass(me, above CallEntry) *int {
	for k := 1; k <= maxCallsToPass; k++ {
		next := me
		next.Right, next.Resolved = me.Right+k, me.Resolved+k
		rate := ratioBps(next.Right, next.Resolved)
		next.RateBps = &rate
		if compareCallEntries(next, above) < 0 {
			return &k
		}
	}
	return nil
}

func ComputeGap(me CallEntry, above, below *CallEntry) Gap {
	versus := above
	out := Gap{Versus: VersusAbove}
	if above == nil {
		versus, out.Versus = below, VersusBelow
	}
	if versus == nil || me.RateBps == nil || versus.RateBps == nil {
		return Gap{}
	}
	out.Points = RatePct(*me.RateBps) - RatePct(*versus.RateBps)
	if out.Versus == VersusAbove {
		out.CallsToPass = CallsToPass(me, *above)
		out.AboveIsPerfect = *above.RateBps == bpsPerUnit
	}
	return out
}
