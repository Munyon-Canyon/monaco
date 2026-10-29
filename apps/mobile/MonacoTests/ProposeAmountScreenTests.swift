import MonacoCore
import Testing

@testable import Monaco

struct ProposeAmountScreenTests {
    @Test func theKeypadStateMirrorsTheModelState() {
        #expect(AmountKeypadState(.empty) == .empty)
        #expect(AmountKeypadState(.typed) == .typed)
        #expect(
            AmountKeypadState(.invalid(limitLine: "Amount exceeds the cash balance.", slot: "Up to $5,100.00"))
                == .invalid(limitLine: "Amount exceeds the cash balance.", slot: "Up to $5,100.00"))
        #expect(AmountKeypadState(.settled(line: "Done")) == .settled(line: "Done"))
    }

    @Test func anInvalidAmountHidesThePresetsAndATypedOneKeepsThem() {
        #expect(!AmountKeypadState(.invalid(limitLine: "Over", slot: "Up to $1.00")).showsPresets)
        #expect(AmountKeypadState(.typed).showsPresets)
    }
}
