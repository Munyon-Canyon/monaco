import Foundation

public struct ProposalOutcome: Equatable, Sendable {
    public static let retention: TimeInterval = 24 * 3600

    public let proposal: ProposalSummary

    init?(from previous: ProposalStatus?, to proposal: ProposalSummary) {
        guard previous == .passed, proposal.myBallot != nil, proposal.isSell,
            proposal.status == .executed || proposal.status == .executionBlocked
        else { return nil }
        self.proposal = proposal
    }

    public func toast(asset: ProposalAsset?) -> String {
        guard proposal.status == .executed else {
            return proposal.statusMessage.map { "Couldn't sell: \($0)" } ?? "Couldn't sell"
        }
        let kind = asset?.kind ?? .stock
        let name = asset?.displayName ?? AssetSymbolFormatter.display(proposal.symbol, kind: kind)
        guard let tokens = proposal.tokenAmount else { return "Sold \(name)" }
        let shares = TokenQuantityFormatter.label(
            fromAtomics: String(tokens), decimals: asset?.decimals ?? ProposalShareFormatter.defaultDecimals,
            kind: kind)
        return "Sold \(shares) of \(name)"
    }
}

extension ProposalSummary {
    var isSell: Bool { kind == "sell" }

    func isRecentOutcome(now: Date) -> Bool {
        let swapFailed = status == .passed && swap?.status == "failed"
        let settled = status == .executed || status == .executionBlocked || swapFailed
        return settled && expiresAt > now.addingTimeInterval(-ProposalOutcome.retention)
    }
}
