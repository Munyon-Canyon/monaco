import Foundation
import MonacoAPI
import Observation

public enum FundSourceCopy {
    public static let title = "Choose a cabal"
    public static let fallbackName = "this cabal"

    public static func cash(micros: Int64) -> String {
        "Cash \(UsdAmountFormatter.format(flooredMicros: micros))"
    }

    public static func yours(bps: Int32) -> String {
        "\(FundCabalCopy.percent(bps: bps)) yours"
    }
}

public struct FundSourceRow: Identifiable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let pictureURL: String?
    public let ownership: String

    public init(id: String, name: String, pictureURL: String? = nil, ownership: String) {
        self.id = id
        self.name = name
        self.pictureURL = pictureURL
        self.ownership = ownership
    }

    init(_ cabal: Components.Schemas.PortfolioCabal) {
        self.init(
            id: cabal.cabal.id, name: cabal.cabal.name, pictureURL: cabal.cabal.pictureUrl,
            ownership: FundSourceCopy.yours(bps: cabal.ownershipBps))
    }
}

@Observable
@MainActor
public final class FundSourceListModel {
    public private(set) var rows: [FundSourceRow] = []
    public private(set) var isLoaded = false

    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func row(for id: String) -> FundSourceRow? {
        rows.first { $0.id == id }
    }

    public func name(for id: String, opener: (id: String, name: String?)?) -> String {
        if let row = row(for: id) { return row.name }
        if let opener, opener.id == id, let name = opener.name?.trimmingCharacters(in: .whitespacesAndNewlines),
            !name.isEmpty
        {
            return name
        }
        return FundSourceCopy.fallbackName
    }

    public func load() async {
        let portfolio = try? await api.read { try await $0.getMyPortfolio().ok.body.json }
        guard !Task.isCancelled else { return }
        rows = portfolio?.cabals.map(FundSourceRow.init) ?? []
        isLoaded = true
    }
}
