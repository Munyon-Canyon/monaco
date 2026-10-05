import MonacoCore
import SwiftUI

/// Activity row rules shared by the group section, the full list, and tests of intent.
enum GroupActivityRules {
    static func canRetry(_ item: GroupActivityItemDTO) -> Bool {
        guard item.status.lowercased() == "failed" else { return false }
        return ["buy", "sell"].contains(item.kind.lowercased())
    }

    static func needsProposalFallback(_ item: GroupActivityItemDTO) -> Bool {
        if isAgentGovernanceKind(item.kind) {
            return true
        }
        return ["buy", "sell"].contains(item.kind.lowercased())
            && item.status.lowercased() == "pending"
            && (item.txSignature ?? "").isEmpty
    }

    static func isAgentGovernanceKind(_ kind: String) -> Bool {
        ["add_agent", "pause_agent", "resume_agent", "revoke_agent"].contains(kind.lowercased())
    }

    static func glyph(for kind: String) -> String {
        switch kind.lowercased() {
        case "buy": "arrow.down"
        case "sell": "arrow.up"
        case "deposit": "plus"
        default: isAgentGovernanceKind(kind) ? "cpu" : "circle"
        }
    }

    /// "Bought Apple", "Buying Apple", "Money added"; agent kinds keep the shared formatter's copy.
    static func title(for item: GroupActivityItemDTO) -> String {
        let status = item.status.lowercased()
        let stock =
            item.symbol.map { AssetDisplayNames.name(forSymbol: $0) ?? AssetSymbolFormatter.display($0) } ?? "stock"
        let byAgent =
            item.initiatedBy?.lowercased() == "agent"
            ? item.agentDisplayName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            : ""
        let prefix = byAgent.isEmpty ? "" : "\(byAgent) · "
        switch item.kind.lowercased() {
        case "deposit":
            return status == "confirmed" ? "Money added" : "Adding money"
        case "buy":
            return prefix
                + (status == "confirmed" ? "Bought \(stock)" : status == "failed" ? "Buy \(stock)" : "Buying \(stock)")
        case "sell":
            return prefix
                + (status == "confirmed" ? "Sold \(stock)" : status == "failed" ? "Sell \(stock)" : "Selling \(stock)")
        default:
            break
        }
        return GroupActivityTitleFormatter.format(
            kind: item.kind,
            symbol: item.symbol,
            agentDisplayName: item.agentDisplayName,
            initiatedBy: item.initiatedBy
        )
    }

    /// Nil when confirmed: a row only says its status when something needs attention.
    static func statusLabel(_ status: String) -> (text: String, isFailure: Bool)? {
        if DepositStatusNormalizer.isConfirmed(status) { return nil }
        if DepositStatusNormalizer.isPending(status) { return ("Pending", false) }
        if DepositStatusNormalizer.isFailed(status) { return ("Failed", true) }
        return (status.capitalized, false)
    }

    /// Dollar figure for the row, or nil when a sell has no proceeds yet (shown in shares instead).
    static func amountMicros(_ item: GroupActivityItemDTO) -> Int64? {
        if item.kind.lowercased() == "sell" {
            if let proceeds = item.proceedsUsdcMicros, let micros = Int64(proceeds), micros > 0 {
                return micros
            }
            // A sell's amountMicros is not dollars; without proceeds the row shows shares instead.
            return nil
        }
        return item.amountMicros
    }

    static func amountLabel(_ item: GroupActivityItemDTO) -> String {
        if item.kind.lowercased() == "sell" {
            if let proceeds = item.proceedsUsdcMicros, let micros = Int64(proceeds), micros > 0 {
                return UsdAmountFormatter.format(micros: micros)
            }
            if let tokenAmount = item.tokenAmount {
                return TokenQuantityFormatter.label(
                    fromAtomics: tokenAmount,
                    decimals: item.resolvedTokenDecimals,
                    kind: item.resolvedAssetKind
                )
            }
            return "—"
        }
        return UsdAmountFormatter.format(micros: item.amountMicros)
    }

    static func sharesLabel(_ shares: Double, kind: AssetKind = .stock) -> String {
        let trimmed = String(format: "%.4f", shares)
            .replacingOccurrences(of: "0+$", with: "", options: .regularExpression)
            .replacingOccurrences(of: "\\.$", with: "", options: .regularExpression)
        if kind == .preIpo {
            return trimmed == "1" ? "1 \(PreIpoCopy.tokenLabelSingular)" : "\(trimmed) \(PreIpoCopy.tokenLabelPlural)"
        }
        return trimmed == "1" ? "1 share" : "\(trimmed) shares"
    }

    static func timeLabel(_ raw: String) -> String {
        RelativeTimeFormatter.label(iso: raw)
    }

    static func parseDate(_ raw: String) -> Date? {
        SharedFormatters.iso8601Date(from: raw)
    }
}
