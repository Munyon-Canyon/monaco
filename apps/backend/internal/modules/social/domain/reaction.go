package domain

import (
	"log/slog"
	"unicode"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	MaxReactionsPerUser = 10
	emojiMaxBytes       = 32
	zwj                 = '\u200D'
	vs16                = '\uFE0F'
	keycap              = '\u20E3'
	heart               = "\u2764"
)

type Emoji string

func ParseEmoji(s string) (Emoji, error) {
	if s == heart {
		s += string(vs16)
	}
	if len(s) == 0 || len(s) > emojiMaxBytes || !oneEmoji([]rune(s)) {
		return "", errs.New(errs.CodeInvalidInput, "social.ParseEmoji", slog.String("field", "emoji"))
	}
	return Emoji(s), nil
}

func (e Emoji) String() string { return string(e) }

func oneEmoji(r []rune) bool {
	switch {
	case isKeycapBase(r[0]):
		return len(r) == 3 && r[1] == vs16 && r[2] == keycap
	case isRegionalIndicator(r[0]):
		return len(r) == 2 && isRegionalIndicator(r[1])
	default:
		return pictographSequence(r)
	}
}

func pictographSequence(r []rune) bool {
	pictographs := extendedPictographic()
	wantPictograph := true
	for _, c := range r {
		switch {
		case unicode.Is(pictographs, c):
			if !wantPictograph {
				return false
			}
			wantPictograph = false
		case c == zwj || c == vs16 || isSkinTone(c) || isTag(c):
			if wantPictograph {
				return false
			}
			wantPictograph = c == zwj
		default:
			return false
		}
	}
	return !wantPictograph
}

func isKeycapBase(c rune) bool { return c >= '0' && c <= '9' || c == '#' || c == '*' }

func isRegionalIndicator(c rune) bool { return c >= 0x1F1E6 && c <= 0x1F1FF }

func isSkinTone(c rune) bool { return c >= 0x1F3FB && c <= 0x1F3FF }

func isTag(c rune) bool { return c >= 0xE0020 && c <= 0xE007F }

func extendedPictographic() *unicode.RangeTable {
	return &unicode.RangeTable{R16: pictographs16(), R32: pictographs32(), LatinOffset: 2}
}

func pictographs16() []unicode.Range16 {
	return []unicode.Range16{
		{0x00A9, 0x00A9, 1},
		{0x00AE, 0x00AE, 1},
		{0x203C, 0x203C, 1},
		{0x2049, 0x2049, 1},
		{0x2122, 0x2122, 1},
		{0x2139, 0x2139, 1},
		{0x2194, 0x2199, 1},
		{0x21A9, 0x21AA, 1},
		{0x231A, 0x231B, 1},
		{0x2328, 0x2328, 1},
		{0x2388, 0x2388, 1},
		{0x23CF, 0x23CF, 1},
		{0x23E9, 0x23F3, 1},
		{0x23F8, 0x23FA, 1},
		{0x24C2, 0x24C2, 1},
		{0x25AA, 0x25AB, 1},
		{0x25B6, 0x25B6, 1},
		{0x25C0, 0x25C0, 1},
		{0x25FB, 0x25FE, 1},
		{0x2600, 0x2605, 1},
		{0x2607, 0x2612, 1},
		{0x2614, 0x2685, 1},
		{0x2690, 0x2705, 1},
		{0x2708, 0x2712, 1},
		{0x2714, 0x2714, 1},
		{0x2716, 0x2716, 1},
		{0x271D, 0x271D, 1},
		{0x2721, 0x2721, 1},
		{0x2728, 0x2728, 1},
		{0x2733, 0x2734, 1},
		{0x2744, 0x2744, 1},
		{0x2747, 0x2747, 1},
		{0x274C, 0x274C, 1},
		{0x274E, 0x274E, 1},
		{0x2753, 0x2755, 1},
		{0x2757, 0x2757, 1},
		{0x2763, 0x2767, 1},
		{0x2795, 0x2797, 1},
		{0x27A1, 0x27A1, 1},
		{0x27B0, 0x27B0, 1},
		{0x27BF, 0x27BF, 1},
		{0x2934, 0x2935, 1},
		{0x2B05, 0x2B07, 1},
		{0x2B1B, 0x2B1C, 1},
		{0x2B50, 0x2B50, 1},
		{0x2B55, 0x2B55, 1},
		{0x3030, 0x3030, 1},
		{0x303D, 0x303D, 1},
		{0x3297, 0x3297, 1},
		{0x3299, 0x3299, 1},
	}
}

func pictographs32() []unicode.Range32 {
	return []unicode.Range32{
		{0x1F000, 0x1F0FF, 1},
		{0x1F10D, 0x1F10F, 1},
		{0x1F12F, 0x1F12F, 1},
		{0x1F16C, 0x1F171, 1},
		{0x1F17E, 0x1F17F, 1},
		{0x1F18E, 0x1F18E, 1},
		{0x1F191, 0x1F19A, 1},
		{0x1F1AD, 0x1F1E5, 1},
		{0x1F201, 0x1F20F, 1},
		{0x1F21A, 0x1F21A, 1},
		{0x1F22F, 0x1F22F, 1},
		{0x1F232, 0x1F23A, 1},
		{0x1F23C, 0x1F23F, 1},
		{0x1F249, 0x1F3FA, 1},
		{0x1F400, 0x1F53D, 1},
		{0x1F546, 0x1F64F, 1},
		{0x1F680, 0x1F6FF, 1},
		{0x1F774, 0x1F77F, 1},
		{0x1F7D5, 0x1F7FF, 1},
		{0x1F80C, 0x1F80F, 1},
		{0x1F848, 0x1F84F, 1},
		{0x1F85A, 0x1F85F, 1},
		{0x1F888, 0x1F88F, 1},
		{0x1F8AE, 0x1F8FF, 1},
		{0x1F90C, 0x1F93A, 1},
		{0x1F93C, 0x1F945, 1},
		{0x1F947, 0x1FAFF, 1},
		{0x1FC00, 0x1FFFD, 1},
	}
}
