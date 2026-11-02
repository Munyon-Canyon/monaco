import Testing

@testable import Monaco

struct PhoneStepPrimaryTests {
    @Test func theNumberStepSendsACode() {
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: false, isCodeStep: false) == .sendCode)
    }

    @Test func theCodeStepContinues() {
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: false, isCodeStep: true) == .link)
    }

    @Test func aNumberLinkedElsewhereOffersSigningInWithItOnBothSteps() {
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: true, isCodeStep: false) == .signInWithNumber)
        #expect(PhoneStepPrimary.resolve(linkedElsewhere: true, isCodeStep: true) == .signInWithNumber)
    }
}
