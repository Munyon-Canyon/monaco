import SwiftUI

enum MonacoIcon: String, CaseIterable {
    case home, cabals, markets, feed, profile
    case bell, settings, filter, search, close, expand
    case chevron, chevronDown, plus, minus, check, checkCircle
    case share, info, chat, heart, star, eye, edit
    case deposit, propose, cashOut, up, down
    case wallet, card, receipt, qr, copy, link, globe
    case clock, alert, offline, refresh, lock
    case mail, phone, bot, pause, play, trash, news, delete

    var symbol: String {
        switch self {
        case .home: return "house"
        case .cabals: return "person.3"
        case .markets: return "chart.line.uptrend.xyaxis"
        case .feed: return "newspaper"
        case .profile: return "person.crop.circle"
        case .bell: return "bell"
        case .settings: return "gearshape"
        case .filter: return "line.3.horizontal.decrease"
        case .search: return "magnifyingglass"
        case .close: return "xmark"
        case .expand: return "arrow.up"
        case .chevron: return "chevron.right"
        case .chevronDown: return "chevron.down"
        case .plus: return "plus"
        case .minus: return "minus"
        case .check: return "checkmark"
        case .checkCircle: return "checkmark.circle.fill"
        case .share: return "square.and.arrow.up"
        case .info: return "info.circle"
        case .chat: return "bubble.left"
        case .heart: return "heart"
        case .star: return "star"
        case .eye: return "eye"
        case .edit: return "pencil"
        case .deposit: return "plus"
        case .propose: return "arrow.up.right"
        case .cashOut: return "arrow.down"
        case .up: return "arrow.up"
        case .down: return "arrow.down"
        case .wallet: return "wallet.bifold"
        case .card: return "creditcard"
        case .receipt: return "doc.text"
        case .qr: return "qrcode"
        case .copy: return "doc.on.doc"
        case .link: return "link"
        case .globe: return "globe"
        case .clock: return "clock"
        case .alert: return "exclamationmark.triangle"
        case .offline: return "wifi.exclamationmark"
        case .refresh: return "arrow.clockwise"
        case .lock: return "lock"
        case .mail: return "envelope"
        case .phone: return "phone"
        case .bot: return "cpu"
        case .pause: return "pause.fill"
        case .play: return "play.fill"
        case .trash: return "trash"
        case .news: return "newspaper"
        case .delete: return "delete.left"
        }
    }

    var filled: String {
        switch self {
        case .home, .cabals, .feed, .profile, .bell, .chat, .heart, .star:
            return symbol.hasSuffix(".fill") ? symbol : symbol + ".fill"
        default:
            return symbol
        }
    }
}

extension Image {
    init(monaco icon: MonacoIcon, filled: Bool = false) {
        self.init(systemName: filled ? icon.filled : icon.symbol)
    }
}

extension MonacoGlassButton {
    init(
        icon: MonacoIcon,
        accessibilityLabel: String,
        style: Style = .glass,
        placement: Placement = .content,
        size: Size = .regular,
        isBusy: Bool = false,
        action: @escaping () -> Void
    ) {
        self.init(
            systemImage: icon.symbol,
            accessibilityLabel: accessibilityLabel,
            style: style,
            placement: placement,
            size: size,
            isBusy: isBusy,
            action: action)
    }
}
