import MonacoAPI

public struct LeaderboardRowView: Equatable, Sendable, Identifiable {
    public let id: String
    public let rank: Int
    public let name: String
    public let handle: String?
    public let pictureURL: String?
    public let valueMicros: Int64
    public let pnlMicros: Int64
    public let returnBps: Int64?
    public let pricesDelayed: Bool
    public internal(set) var isViewer = false

    init(
        id: String, rank: Int, name: String, handle: String?, pictureURL: String?, valueMicros: Int64,
        pnlMicros: Int64, returnBps: Int64?, pricesDelayed: Bool, isViewer: Bool = false
    ) {
        self.id = id
        self.rank = rank
        self.name = name
        self.handle = handle
        self.pictureURL = pictureURL
        self.valueMicros = valueMicros
        self.pnlMicros = pnlMicros
        self.returnBps = returnBps
        self.pricesDelayed = pricesDelayed
        self.isViewer = isViewer
    }

    public init(_ row: Components.Schemas.LeaderboardRow) {
        self.init(
            id: row.subject.id, rank: Int(row.rank), name: row.subject.name, handle: row.subject.handle,
            pictureURL: row.subject.pictureUrl, valueMicros: row.valueMicros, pnlMicros: row.pnlMicros,
            returnBps: row.returnBps, pricesDelayed: row.flags.contains(.stalePrices))
    }

    init(me row: Components.Schemas.LeaderboardPage.MePayload) {
        self.init(
            id: row.subject.id, rank: Int(row.rank), name: row.subject.name, handle: row.subject.handle,
            pictureURL: row.subject.pictureUrl, valueMicros: row.valueMicros, pnlMicros: row.pnlMicros,
            returnBps: row.returnBps, pricesDelayed: row.flags.contains(.stalePrices), isViewer: true)
    }

    var key: BoardRowKey { BoardRowKey(id: id, rank: rank) }
}
