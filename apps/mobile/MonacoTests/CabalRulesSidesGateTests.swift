import Testing

@testable import Monaco

struct CabalRulesSidesGateTests {
    @Test
    func aFlipOpensTheSheetOnce() {
        #expect(CabalRulesSidesGate.shouldOpen(requested: true, current: false, isOpen: false))
        #expect(!CabalRulesSidesGate.shouldOpen(requested: true, current: false, isOpen: true))
    }

    @Test
    func asksForTheValueTheRuleAlreadyHasDoNothing() {
        #expect(!CabalRulesSidesGate.shouldOpen(requested: false, current: false, isOpen: false))
        #expect(!CabalRulesSidesGate.shouldOpen(requested: true, current: true, isOpen: false))
    }

    @Test
    func turningItOffOpensTheSheetToo() {
        #expect(CabalRulesSidesGate.shouldOpen(requested: false, current: true, isOpen: false))
    }
}
