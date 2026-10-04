import Foundation

extension Components.Schemas.Proposal {
    public static func sample(
        status: Components.Schemas.ProposalStatus = .open, kind: Components.Schemas.ProposalKind = .buy
    ) -> Self {
        .init(
            id: "proposal-1", cabalId: "cabal-1", proposerId: "user-1", kind: kind, symbol: "AAPLx",
            usdcMicros: 1_000_000,
            quoteOutAmount: 1, status: status, expiresAt: .now.addingTimeInterval(86_400), createdAt: .now,
            tally: .init(yes: 0, no: 0, voters: 3, needed: 2))
    }
}
