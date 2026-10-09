import Foundation

public struct ProposalOutcome: Equatable, Sendable {
    public static let retention: TimeInterval = 24 * 3600

    public let proposal: ProposalSummary

    init?(from previous: ProposalStatus?, to proposal: ProposalSummary) {
        guard previous == .open || previous == .passed, proposal.myBallot != nil,
            proposal.status == .executed || proposal.status == .executionBlocked
        else { return nil }
        self.proposal = proposal
    }

    public func toast(asset: ProposalAsset?) -> String {
        guard proposal.status == .executed else {
            let action = proposal.isSell ? "sell" : "buy"
            return proposal.statusMessage.map { "Couldn't \(action): \($0)" } ?? "Couldn't \(action)"
        }
        let kind = asset?.kind ?? .stock
        let name = asset?.displayName ?? AssetSymbolFormatter.display(proposal.symbol, kind: kind)
        guard proposal.isSell else {
            guard let usdcMicros = proposal.usdcMicros else { return "Bought \(name)" }
            return "Bought \(UsdAmountFormatter.format(micros: usdcMicros)) of \(name)"
        }
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
