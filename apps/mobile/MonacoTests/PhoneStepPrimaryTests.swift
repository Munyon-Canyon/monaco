import Testing

@testable import Monaco

struct PhoneStepPrimaryTests {
    @Test func theNumberStepSendsACode() {
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: false, isCodeStep: false) == .sendCode)
    }

    @Test func theCodeStepContinues() {
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: false, isCodeStep: true) == .link)
    }

    @Test func aNumberLinkedElsewhereOffersSkipOnBothSteps() {
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: true, isCodeStep: false) == .skip)
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: true, isCodeStep: true) == .skip)
    }

    @Test func skipIsInTheToolbarOnlyWhenItIsNotTheCapsule() {
        #expect(PhoneStepPrimary.sendCode.showsToolbarSkip)
        #expect(PhoneStepPrimary.link.showsToolbarSkip)
        #expect(!PhoneStepPrimary.skip.showsToolbarSkip)
    }
}
