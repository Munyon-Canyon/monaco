import Foundation
import MonacoAPI
import Observation

extension CatchUpScreen {
    public var activeItems: [Item] { waiting + replies + invites + results }

    public func activeItem(key: String) -> Item? { slot(of: key)?.item }
}

public struct CatchUpDeck: Equatable, Sendable {
    public typealias Item = Components.Schemas.CatchUpItem

    public static let upNextLimit = 4

    public private(set) var items: [Item]
    public private(set) var cursor: Int
    private var gone: Set<String> = []

    public init(items: [Item], startingAt key: String? = nil) {
        self.items = items
        if let key, let index = items.firstIndex(where: { $0.key == key }) {
            cursor = index
        } else {
            cursor = items.firstIndex(where: \.unread) ?? 0
        }
    }

    public init(screen: CatchUpScreen, startingAt key: String? = nil) {
        self.init(items: screen.activeItems, startingAt: key)
    }

    public var total: Int { items.count }

    public var isFinished: Bool { cursor >= items.count }

    public var current: Item? { isFinished ? nil : items[cursor] }

    public var position: String { "\(min(cursor + 1, max(total, 1))) of \(total)" }

    public var upNext: [Item] {
        guard !isFinished else { return [] }
        return Array(items[(cursor + 1)...].filter { !gone.contains($0.key) }.prefix(Self.upNextLimit))
    }

    public mutating func advance() {
        guard !isFinished else { return }
        cursor += 1
        skipGone()
    }

    public mutating func advance(past key: String) {
        guard current?.key == key else { return }
        advance()
    }

    public mutating func reconcile(with screen: CatchUpScreen) {
        for index in items.indices {
            if let live = screen.activeItem(key: items[index].key) {
                items[index] = live
                gone.remove(live.key)
            } else {
                gone.insert(items[index].key)
            }
        }
        skipGone()
    }

    private mutating func skipGone() {
        while let item = current, gone.contains(item.key) { cursor += 1 }
    }
}

@Observable
@MainActor
public final class CatchUpDeckModel {
    public private(set) var deck: CatchUpDeck

    private let catchUp: CatchUpModel

    public init(catchUp: CatchUpModel, screen: CatchUpScreen, startingAt key: String? = nil) {
        self.catchUp = catchUp
        deck = CatchUpDeck(screen: screen, startingAt: key)
    }

    public func sync() {
        guard case .loaded(let screen) = catchUp.state else { return }
        deck.reconcile(with: screen)
    }

    @discardableResult
    public func keep() async -> Bool {
        guard let key = deck.current?.key else { return false }
        deck.advance(past: key)
        return await catchUp.keep(key)
    }

    @discardableResult
    public func clear() async -> Bool {
        guard let key = deck.current?.key else { return false }
        deck.advance(past: key)
        return await catchUp.clear(key)
    }
}
