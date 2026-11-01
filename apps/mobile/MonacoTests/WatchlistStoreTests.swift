import Foundation
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct WatchlistStoreTests {
    private func suite() throws -> UserDefaults {
        let name = "watchlist-tests-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: name))
        defaults.removePersistentDomain(forName: name)
        return defaults
    }

    @Test func aStarSurvivesAnAppRelaunch() throws {
        let defaults = try suite()
        let first = WatchlistModel(userID: "u1", store: UserDefaultsWatchlistStore(defaults: defaults))
        first.toggle("NVDAx")
        first.toggle("AAPLx")

        let relaunched = WatchlistModel(userID: "u1", store: UserDefaultsWatchlistStore(defaults: defaults))

        #expect(relaunched.symbols == ["NVDAx", "AAPLx"])
    }

    @Test func eachUserKeepsTheirOwnListOnTheSameDevice() throws {
        let defaults = try suite()
        let store = UserDefaultsWatchlistStore(defaults: defaults)
        WatchlistModel(userID: "u1", store: store).toggle("NVDAx")

        #expect(WatchlistModel(userID: "u2", store: store).symbols.isEmpty)
        #expect(defaults.data(forKey: "markets-watchlist.u1") != nil)
        #expect(defaults.data(forKey: "markets-watchlist.u2") == nil)
    }

    @Test func theListIsStoredAsAJSONArrayOfSymbols() throws {
        let defaults = try suite()
        WatchlistModel(userID: "u1", store: UserDefaultsWatchlistStore(defaults: defaults)).toggle("NVDAx")

        let data = try #require(defaults.data(forKey: "markets-watchlist.u1"))

        #expect(String(decoding: data, as: UTF8.self) == #"["NVDAx"]"#)
    }

    @Test func oneModelServesEverySurfaceOfTheSameUser() throws {
        let models = WatchlistModels(store: UserDefaultsWatchlistStore(defaults: try suite()))

        let markets = models.model(for: "u1")
        let stock = models.model(for: "u1")
        markets.toggle("NVDAx")

        #expect(markets === stock)
        #expect(stock.isWatched("NVDAx"))
        #expect(models.model(for: "u2") !== markets)
        #expect(models.model(for: "u2").symbols.isEmpty)
    }

    @Test func aSessionWithNoUserKeepsItsStarsOnlyInMemory() throws {
        let defaults = try suite()
        let models = WatchlistModels(store: UserDefaultsWatchlistStore(defaults: defaults))

        models.model(for: nil).toggle("NVDAx")

        #expect(models.model(for: nil).isWatched("NVDAx"))
        #expect(defaults.dictionaryRepresentation().keys.filter { $0.hasPrefix("markets-watchlist.") }.isEmpty)
    }

    @Test func starsAddedBeforeTheViewerLoadsLandOnTheirList() throws {
        let defaults = try suite()
        let models = WatchlistModels(store: UserDefaultsWatchlistStore(defaults: defaults))
        models.model(for: nil).toggle("NVDAx")
        models.model(for: nil).toggle("AAPLx")

        let viewer = models.model(for: "u1")

        #expect(viewer.symbols == ["NVDAx", "AAPLx"])
        #expect(models.model(for: nil).symbols.isEmpty)
        let relaunched = WatchlistModel(userID: "u1", store: UserDefaultsWatchlistStore(defaults: defaults))
        #expect(relaunched.symbols == ["NVDAx", "AAPLx"])
    }

    @Test func aQueuedStarNeverDuplicatesOneTheViewerAlreadyHas() throws {
        let defaults = try suite()
        WatchlistModel(userID: "u1", store: UserDefaultsWatchlistStore(defaults: defaults)).toggle("NVDAx")
        let models = WatchlistModels(store: UserDefaultsWatchlistStore(defaults: defaults))
        models.model(for: nil).toggle("NVDAx")
        models.model(for: nil).toggle("MSFTx")

        let viewer = models.model(for: "u1")

        #expect(viewer.symbols == ["NVDAx", "MSFTx"])
    }
}
