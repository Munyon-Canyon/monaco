import Foundation

public struct PushSample: Equatable, Sendable {
    public let title: String
    public let body: String
    public let cabalImageURL: URL?

    public init(title: String, body: String, cabalImageURL: URL? = nil) {
        self.title = title
        self.body = body
        self.cabalImageURL = cabalImageURL
    }

    public static func afterVote(
        symbol: String, status: ProposalStatus, tally: ProposalTally, isSell: Bool, usdcMicros: Int64?,
        closesAt: Date, swap: ProposalSwap?, pendingUnits: String?, cabalImageURL: URL? = nil,
        calendar: Calendar = .current
    ) -> PushSample {
        let ticker = symbol.hasSuffix("x") ? String(symbol.dropLast()) : symbol
        let score = "\(tally.yes)–\(tally.no)"
        switch ProposalResultCopy.seal(status: status) {
        case .passed:
            let body =
                swap.flatMap { ProposalResultCopy.filled($0, unit: .shares, isSell: isSell) }.map { $0 + "." }
                ?? pendingUnits.map { "\(isSell ? "Selling" : "Buying") \($0) sh now." }
                ?? "\(isSell ? "Selling" : "Buying") now."
            return PushSample(title: "\(ticker) passed \(score)", body: body, cabalImageURL: cabalImageURL)
        case .failed:
            return PushSample(title: "\(ticker) failed \(score)", body: "No trade.", cabalImageURL: cabalImageURL)
        case .neutral:
            let clock = SharedFormatters.string(
                from: closesAt, pattern: .fixed("h:mm a"), locale: Locale(identifier: "en_US_POSIX"),
                calendar: calendar)
            let amount =
                usdcMicros.map { " " + UsdAmountFormatter.format(micros: $0).replacingOccurrences(of: ".00", with: "") }
                ?? ""
            return PushSample(
                title: "\(isSell ? "Sell" : "Buy") \(ticker)\(amount) · closes \(clock)",
                body: "We'll tell you how it ends.", cabalImageURL: cabalImageURL)
        }
    }
}
