import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct CabalInPlaceJoinTests {
    @Test func approvingARequestTellsTheScreenToRereadIt() async {
        var cabal = Components.Schemas.Cabal.sample(role: "creator")
        cabal.rules.joinMode = "request"
        let model = CabalAccessModel.preview(cabal: cabal)
        await model.load()
        guard case .pending(let requests) = model.standing, let first = requests.first else {
            Issue.record("expected pending requests, got \(model.standing)")
            return
        }

        await model.decide(first, approve: true)

        #expect(model.membershipChanges == 1)
    }

    @Test func requestingOrDenyingLeavesTheMembersAsTheyWere() async {
        var outsider = Components.Schemas.Cabal.sample(role: nil)
        outsider.rules.joinMode = "request"
        let requester = CabalAccessModel.preview(cabal: outsider)
        await requester.load()
        await requester.enter()
        #expect(requester.toast?.message == CabalEntry.requestedToast)
        #expect(requester.membershipChanges == 0)

        var owned = Components.Schemas.Cabal.sample(role: "creator")
        owned.rules.joinMode = "request"
        let creator = CabalAccessModel.preview(cabal: owned)
        await creator.load()
        guard case .pending(let requests) = creator.standing, let first = requests.first else {
            Issue.record("expected pending requests, got \(creator.standing)")
            return
        }
        await creator.decide(first, approve: false)
        #expect(creator.membershipChanges == 0)
    }
}
