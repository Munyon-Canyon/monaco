import Foundation
import MonacoAPI

public struct AccountActivityRow: Identifiable, Equatable, Sendable {
    public enum Status: Equatable, Sendable {
        case pending
        case settled
        case failed

        public var rowLabel: String? {
            switch self {
            case .pending: "Pending"
            case .settled: nil
            case .failed: "Failed"
            }
        }

        public var receiptLabel: String {
            switch self {
            case .pending: "Pending"
            case .settled: "Done"
            case .failed: "Failed"
            }
        }
    }

    public struct Cabal: Equatable, Sendable {
        public let id: String
        public let name: String
    }

    public let id: String
    public let title: String
    public let glyph: String
    public let date: String
    public let fullDate: String
    public let status: Status
    public let amount: String
    public let cabal: Cabal?
    public let solscanURL: URL?

    public init(_ txn: Components.Schemas.UserTxn, now: Date, timeZone: TimeZone = .current) throws {
        guard let micros = Int64(txn.usdcMicros) else { throw APIError.decoding("usdc_micros") }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        let cabal = txn.cabal.map { Cabal(id: $0.id, name: $0.name) }
        let sameYear = calendar.component(.year, from: txn.createdAt) == calendar.component(.year, from: now)
        let headline = Self.headline(txn.kind, cabalName: cabal?.name ?? "a cabal")
        self.id = txn.id
        self.title = headline.title
        self.glyph = headline.glyph
        self.date = Self.string(txn.createdAt, sameYear ? "MMM d, h:mm a" : "MMM d, yyyy", calendar)
        self.fullDate = Self.string(txn.createdAt, "MMM d, yyyy 'at' h:mm a", calendar)
        self.status = Status(txn.status)
        self.amount = UsdAmountFormatter.format(signedMicros: micros)
        self.cabal = cabal
        self.solscanURL = txn.txSignature.flatMap { URL(string: "https://solscan.io/tx/\($0)") }
    }

    private static func headline(
        _ kind: Components.Schemas.UserTxn.KindPayload, cabalName: String
    ) -> (title: String, glyph: String) {
        switch kind {
        case .deposit: ("Deposit", "arrow.down.to.line")
        case .withdrawal: ("Withdrawal", "arrow.up.right")
        case .fund: ("Funded \(cabalName)", "plus")
        case .cashOut: ("Cashed out of \(cabalName)", "arrow.down.left")
        }
    }

    private static func string(_ date: Date, _ pattern: String, _ calendar: Calendar) -> String {
        SharedFormatters.string(
            from: date, pattern: .fixed(pattern), locale: Locale(identifier: "en_US_POSIX"), calendar: calendar)
    }
}

extension AccountActivityRow.Status {
    fileprivate init(_ payload: Components.Schemas.UserTxn.StatusPayload) {
        switch payload {
        case .pending: self = .pending
        case .settled: self = .settled
        case .failed: self = .failed
        }
    }
}
