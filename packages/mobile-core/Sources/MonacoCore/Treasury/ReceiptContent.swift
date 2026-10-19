import Foundation

public struct ReceiptContent: Equatable, Sendable {
    public enum Mark: Equatable, Sendable {
        case glyph(String)
        case stock(symbol: String, logoURL: URL?)
    }

    public enum Status: Equatable, Sendable {
        case completed
        case pending
        case failed
        case unknown
    }

    public enum Tone: Equatable, Sendable {
        case brand
        case warn
        case negative
    }

    public struct Notice: Equatable, Sendable {
        public let tone: Tone
        public let message: String
    }

    public enum Field: Hashable, Sendable {
        case from
        case to
        case asset
        case requestedAmount
        case when
        case status
        case recordID
    }

    public struct KeyRow: Equatable, Sendable {
        public let field: Field
        public let label: String
        public let value: String
        public let isData: Bool
    }

    public enum Related: Equatable, Sendable {
        case accountActivity
        case proposal(String)
    }

    public let mark: Mark
    public let eyebrow: String
    public let amount: String
    public let status: Status
    public let isPersonal: Bool
    public let keyRows: [KeyRow]
    public let cabal: AccountActivityRow.Cabal?
    public let related: Related?
    public let explorerURL: URL?

    public init(
        mark: Mark, eyebrow: String, amount: String, status: Status, isPersonal: Bool, keyRows: [KeyRow],
        cabal: AccountActivityRow.Cabal? = nil, related: Related? = nil, explorerURL: URL? = nil
    ) {
        self.mark = mark
        self.eyebrow = eyebrow
        self.amount = amount
        self.status = status
        self.isPersonal = isPersonal
        self.keyRows = keyRows
        self.cabal = cabal
        self.related = related
        self.explorerURL = explorerURL
    }

    public init(_ row: AccountActivityRow) {
        let ends = Self.ends(row)
        self.init(
            mark: .glyph(row.glyph), eyebrow: row.title, amount: row.amount, status: Status(row.status),
            isPersonal: true,
            keyRows: Self.keyRows(
                from: ends.from, to: ends.to, asset: "USDC", amount: row.amount, when: row.fullDate,
                status: Status(row.status), recordID: row.id),
            cabal: row.kind == .fund || row.kind == .cashOut ? row.cabal : nil, related: .accountActivity,
            explorerURL: row.solscanURL)
    }

    public static func keyRows(
        from: String, to: String, asset: String, amount: String, when: String, status: Status, recordID: String
    ) -> [KeyRow] {
        [
            KeyRow(field: .from, label: "From", value: from, isData: false),
            KeyRow(field: .to, label: "To", value: to, isData: false),
            KeyRow(field: .asset, label: "Asset", value: asset, isData: false),
            KeyRow(field: .requestedAmount, label: "Requested amount", value: amount, isData: true),
            KeyRow(field: .when, label: "When", value: when, isData: true),
            KeyRow(field: .status, label: "Status", value: status.rowValue, isData: false),
            KeyRow(field: .recordID, label: "Record ID", value: recordID, isData: true),
        ]
    }

    private static func ends(_ row: AccountActivityRow) -> (from: String, to: String) {
        let cabalName = row.cabal?.name ?? "a cabal"
        return switch row.kind {
        case .deposit: ("External wallet", "Your cash")
        case .withdrawal: ("Your cash", "External wallet")
        case .fund: ("Your cash", cabalName)
        case .cashOut: (cabalName, "Your cash")
        }
    }

    public var badgeText: String { status.badgeText(isPersonal: isPersonal) }

    public var badgeTone: Tone { status.tone }

    public var notice: Notice? { status.notice(isPersonal: isPersonal) }

    public var canOpenExplorer: Bool { explorerURL != nil }
}

extension ReceiptContent.Status {
    init(_ status: AccountActivityRow.Status) {
        switch status {
        case .settled: self = .completed
        case .pending: self = .pending
        case .failed: self = .failed
        }
    }

    public var rowValue: String {
        switch self {
        case .completed: "Done"
        case .pending: "Pending"
        case .failed: "Not completed"
        case .unknown: "Unknown"
        }
    }

    public var tone: ReceiptContent.Tone {
        switch self {
        case .completed: .brand
        case .pending, .unknown: .warn
        case .failed: .negative
        }
    }

    public func badgeText(isPersonal: Bool) -> String {
        switch self {
        case .completed: "Completed"
        case .pending: isPersonal ? "Processing" : "Execution pending"
        case .failed: "Not completed"
        case .unknown: "Status unconfirmed"
        }
    }

    public func notice(isPersonal: Bool) -> ReceiptContent.Notice? {
        switch self {
        case .completed:
            nil
        case .pending:
            ReceiptContent.Notice(
                tone: .warn,
                message: isPersonal
                    ? "The transfer is processing. The funds are not confirmed at the destination."
                    : "The vote passed. Execution has not been confirmed.")
        case .failed:
            ReceiptContent.Notice(
                tone: .negative,
                message: "This operation did not complete. Check the latest record before submitting another request.")
        case .unknown:
            ReceiptContent.Notice(
                tone: .warn, message: "We couldn’t confirm the latest status. This is not proof of failure.")
        }
    }
}

#if DEBUG
extension ReceiptContent {
    public func with(status: Status) -> ReceiptContent {
        ReceiptContent(
            mark: mark, eyebrow: eyebrow, amount: amount, status: status, isPersonal: isPersonal,
            keyRows: keyRows.map { row in
                row.field == .status
                    ? KeyRow(field: .status, label: row.label, value: status.rowValue, isData: false) : row
            }, cabal: cabal, related: related, explorerURL: explorerURL)
    }

    public static func sampleTrade(status: Status) -> ReceiptContent {
        let eyebrow = status == .completed ? "Bought NVIDIA" : "Buy NVIDIA"
        return ReceiptContent(
            mark: .stock(symbol: "NVDAx", logoURL: nil), eyebrow: eyebrow, amount: "$400.00", status: status,
            isPersonal: false,
            keyRows: keyRows(
                from: "Big Ballers", to: "Cabal holdings", asset: "NVIDIA · NVDA", amount: "$400.00",
                when: "Oct 3, 2026 at 3:00\u{202F}PM", status: status, recordID: "trade-sample-record"),
            related: .proposal("sample-proposal"),
            explorerURL: status == .pending ? nil : URL(string: "https://solscan.io/tx/sample-signature"))
    }
}
#endif
