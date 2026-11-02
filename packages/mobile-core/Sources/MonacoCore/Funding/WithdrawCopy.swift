import Foundation

public struct WithdrawToast: Equatable, Sendable {
    public struct Link: Equatable, Sendable {
        public let title: String
        public let url: URL
    }

    public let message: String
    public let isSuccess: Bool
    public let link: Link?
}

public enum WithdrawCopy {
    public static let assetLabel = "USDC"
    public static let addAddress = "Add an address"
    public static let feeFailed = "Couldn't get the fee. Try again."
    public static let invalidAddress = "That address can't receive USDC from here."
    public static let checkAgain = "Check again"
    public static let processing = "Withdrawal processing · Pending"
    public static let switchToBase = "Switch to Base"
    public static let baseAddressOnSolana = "That's a Base address, not a Solana one."
    public static let ownDeposit = "That's your own Monaco deposit address. Paste the address you want to send to."

    public static func dollars(_ micros: Int64) -> String {
        let text = UsdAmountFormatter.format(micros: micros)
        return text.hasSuffix(".00") ? String(text.dropLast(3)) : text
    }

    private static func flooredDollars(_ micros: Int64) -> String {
        dollars(UsdAmountFormatter.flooredToCents(micros))
    }

    public static func checkPending(amountMicros: Int64) -> String {
        "Check \(dollars(amountMicros)) again"
    }

    public static func pendingBlocked(amountMicros: Int64, address: String) -> String {
        "Your \(dollars(amountMicros)) withdrawal to \(SolanaAddress.shortened(address)) may still be going through. "
            + "Check it again before sending a different one."
    }

    public static func fee(_ micros: Int64) -> String {
        UsdAmountFormatter.format(micros: micros)
    }

    public static func emptyLine(feeMicros: Int64?, network: WithdrawNetwork) -> String {
        guard let feeMicros else { return "Fee shown once you type · \(assetLabel) on \(network.name)" }
        let lead = feeMicros == 0 ? "No fee" : "Fee \(fee(feeMicros))"
        return "\(lead) · \(assetLabel) on \(network.name)"
    }

    public static func typedLine(receiveMicros: Int64, feeMicros: Int64?, isEstimate: Bool = false) -> String {
        let receive = "\(isEstimate ? "≈ " : "")\(units(receiveMicros))"
        guard let feeMicros else { return receive }
        return feeMicros == 0 ? "\(receive) · no fee" : "\(receive) · fee \(units(feeMicros))"
    }

    private static func units(_ micros: Int64) -> String {
        "\(UsdAmountFormatter.format(micros: micros).dropFirst()) \(assetLabel)"
    }

    public static func limitLine(maxMicros: Int64, overMicros: Int64) -> String {
        "You can send \(flooredDollars(maxMicros)) · \(dollars(overMicros)) over"
    }

    public static func limitSlot(maxMicros: Int64) -> String {
        "Up to \(flooredDollars(maxMicros))"
    }

    public static func belowMinimum(minMicros: Int64) -> String {
        "The minimum is \(dollars(minMicros))"
    }

    public static func button(amountMicros: Int64) -> String {
        "Withdraw \(dollars(amountMicros))"
    }

    public static func settledLine(network: WithdrawNetwork, arrivalSeconds: Int) -> String {
        "Sent just now · arrives on \(network.name) in \(wait(arrivalSeconds))"
    }

    public static func arrivalNote(network: WithdrawNetwork, arrivalSeconds: Int) -> String {
        "Arrives on \(network.name) in \(wait(arrivalSeconds))."
    }

    private static func wait(_ arrivalSeconds: Int) -> String {
        arrivalSeconds > 90 ? "about \((arrivalSeconds + 30) / 60) minutes" : "about a minute"
    }

    public static func settledCheck(amountMicros: Int64, address: String) -> String {
        "\(dollars(amountMicros)) to \(SolanaAddress.shortened(address))"
    }

    public static func fullBalanceLabel(micros: Int64) -> String {
        "\(UsdAmountFormatter.format(flooredMicros: micros)) (full balance)"
    }

    public static func addressMessage(for problem: WithdrawAddressProblem, network: WithdrawNetwork) -> String {
        switch problem {
        case .solana(.notAnAccountAddress): "That isn't a Solana address. Check you copied the whole thing."
        case .solana(.ownDepositAddress): ownDeposit
        default: network.message(for: problem)
        }
    }

    public static func toast(for progress: WithdrawProgress, fullBalanceLabel: String?) -> WithdrawToast? {
        guard let line = message(for: progress, fullBalanceLabel: fullBalanceLabel) else { return nil }
        guard case .confirmed(let withdrawal) = progress else {
            return WithdrawToast(message: line, isSuccess: false, link: nil)
        }
        let link = withdrawal.solscanURL.map { WithdrawToast.Link(title: "View on Solscan", url: $0) }
        return WithdrawToast(message: line, isSuccess: true, link: link)
    }

    private static func message(for progress: WithdrawProgress, fullBalanceLabel: String?) -> String? {
        guard let line = progress.toast else { return nil }
        let sent: Withdrawal
        switch progress {
        case .submitted(let withdrawal), .confirmed(let withdrawal): sent = withdrawal
        case .idle, .submitting, .failed: return line
        }
        guard let fullBalanceLabel else { return line }
        return line.replacingOccurrences(
            of: UsdAmountFormatter.format(micros: sent.amountMicros), with: fullBalanceLabel)
    }
}
