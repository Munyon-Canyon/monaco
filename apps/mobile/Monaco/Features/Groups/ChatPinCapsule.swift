import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatPinMetrics {
    static let height: CGFloat = 40
    static let voteHeight: CGFloat = 56
    static let voteMark: CGFloat = 32
    static let voteButtonHeight: CGFloat = 36
    static let sliverDrops: [CGFloat] = [4, 8]
    static let sliverInsets: [CGFloat] = [8, 16]
    static let sideMargin: CGFloat = 16
    static let topGap: CGFloat = 8
    static let minTarget: CGFloat = 44
    static let listRowLines = 3
}

enum ChatPinCopy {
    static let sheetTitle = "Pinned"
    static let messagesTitle = "Messages"
    static let pin = "Pin"
    static let unpin = "Unpin"
    static let unpinned = "Unpinned"
    static let undo = "Undo"
    static let nextPin = "Next pin"
    static let showAll = "Show all pins"
    static let vote = "Vote"
    static let goToVote = "Go to vote"

    static func showsSlivers(itemCount: Int) -> Bool { itemCount > 1 }

    static func counter(index: Int, count: Int) -> String? {
        count > 1 ? "\(index + 1) of \(count)" : nil
    }

    static func firstName(_ displayName: String?) -> String {
        let name = displayName?.split(whereSeparator: \.isWhitespace).first.map(String.init) ?? ""
        return name.isEmpty ? "Someone" : name
    }

    static func pinner(of message: ChatMessage) -> String {
        firstName(message.pinnedBy?.displayName)
    }

    static func text(of message: ChatMessage, cabalName: String, viewerID: String) -> String {
        if let moment = ChatMoment(message: message, viewerID: viewerID) {
            return moment.lines(
                cabalName: cabalName, currentPriceMicros: nil, now: message.createdAt, calendar: .current,
                locale: .current
            ).title
        }
        return (message.body ?? "").split(whereSeparator: \.isWhitespace).joined(separator: " ")
    }

    static func spokenLabel(pinner: String, text: String, counter: String?) -> String {
        ["Pinned by \(pinner): \(text).", counter.map { "\($0)." }].compactMap { $0 }.joined(separator: " ")
    }
}

struct ChatPinControls {
    let pins: ChatPinsModel
    let role: String?
    let viewerID: String

    func canPin(_ message: ChatMessage) -> Bool {
        pins.canPin(message, role: role, viewerID: viewerID)
    }

    func isPinned(_ message: ChatMessage) -> Bool {
        pins.pins.contains { $0.id == message.id }
    }

    @MainActor func pin(_ message: ChatMessage) {
        Task { await pins.pin(message) }
    }

    @MainActor func unpin(_ message: ChatMessage, toasts: ToastCenter) {
        let pins = pins
        Task { await pins.unpin(message) }
        toasts.current = MonacoToast(
            message: ChatPinCopy.unpinned, isSuccess: true,
            action: MonacoToastAction(title: ChatPinCopy.undo) { Task { await pins.pin(message) } })
    }
}

extension EnvironmentValues {
    @Entry var chatPinControls: ChatPinControls?
}

struct ChatPinVote: Equatable {
    let id: String
    let headline: String
    let line: String
    let spoken: String
    let isRule: Bool
    let symbol: String
    let assetName: String?
    let logoURL: URL?
    let showsVote: Bool

    @MainActor init(summary: ProposalSummary, headline: String, asset: ProposalAsset?, now: Date) {
        let ballot = summary.myBallot.flatMap(ProposalBallot.init(rawValue:))
        let voted = ballot.map { "you voted \($0.label)" }
        id = summary.id
        self.headline = headline
        line = [ChatOpenVotesModel.capsuleLine(for: summary, now: now), voted]
            .compactMap { $0 }.joined(separator: " \u{00B7} ")
        spoken =
            [
                "Open vote.",
                Self.spokenHeadline(summary, headline: headline) + ".",
                ChatOpenVotesModel.spokenLine(for: summary, now: now) + ".",
                voted.map { $0.prefix(1).uppercased() + $0.dropFirst() + "." },
            ].compactMap { $0 }.joined(separator: " ")
        isRule = summary.isRule
        symbol = summary.symbol
        assetName = asset?.displayName
        logoURL = asset?.logoURL
        showsVote = summary.canVote && ballot == nil
    }

    static func spokenHeadline(_ summary: ProposalSummary, headline: String) -> String {
        let parts = headline.components(separatedBy: " \u{00B7} ")
        guard summary.kind == "buy", let micros = summary.usdcMicros, let name = parts.first else {
            return parts.joined(separator: ", ")
        }
        let cents = micros / 10_000
        let whole = (cents / 100).formatted()
        let dollars = cents % 100 == 0 ? whole : "\(whole).\(String(format: "%02d", Int(cents % 100)))"
        return "\(name), \(dollars) dollars"
    }
}

enum ChatPinVoteTarget: Equatable {
    case scroll(String)
    case proposal(String)

    static func resolve(proposalID: String, rows: [ChatRow]) -> ChatPinVoteTarget {
        rows.first { $0.message.proposalId == proposalID }.map { .scroll($0.id) } ?? .proposal(proposalID)
    }
}

struct ChatPinCapsule: View {
    let pins: ChatPinsModel
    var vote: ChatPinVote?
    var itemCount = 0
    let cabalName: String
    let viewerID: String
    let next: () -> Void
    let showAll: () -> Void
    var castVote: () -> Void = {}
    var goToVote: () -> Void = {}

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var message: ChatMessage? {
        pins.pins.indices.contains(pins.current) ? pins.pins[pins.current] : nil
    }

    private var showsSlivers: Bool { ChatPinCopy.showsSlivers(itemCount: max(itemCount, pins.pins.count)) }

    var body: some View {
        content
            .animation(reduceMotion ? nil : .easeInOut(duration: 0.25), value: vote?.id)
    }

    @ViewBuilder private var content: some View {
        if let vote {
            HStack(spacing: 0) {
                voteBody(vote)
                if vote.showsVote { voteButton }
                listButton(height: ChatPinMetrics.voteHeight)
            }
            .frame(minHeight: ChatPinMetrics.voteHeight)
            .modifier(ChatPinCapsuleSurface())
            .modifier(ChatPinCapsuleLayout(slivers: showsSlivers))
            .transition(.opacity)
            .padding(.top, ChatPinMetrics.topGap)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("chat-pin-vote")
        } else if let message {
            let pinner = ChatPinCopy.pinner(of: message)
            let text = ChatPinCopy.text(of: message, cabalName: cabalName, viewerID: viewerID)
            let counter = ChatPinCopy.counter(index: pins.current, count: pins.pins.count)
            HStack(spacing: 0) {
                nextButton(pinner: pinner, text: text, counter: counter)
                listButton(height: ChatPinMetrics.height)
            }
            .frame(minHeight: ChatPinMetrics.height)
            .modifier(ChatPinCapsuleSurface())
            .modifier(ChatPinCapsuleLayout(slivers: showsSlivers))
            .transition(.opacity)
            .padding(.top, ChatPinMetrics.topGap)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("chat-pin-capsule")
        }
    }

    private func voteBody(_ vote: ChatPinVote) -> some View {
        Button(action: goToVote) {
            HStack(spacing: MonacoTheme.Space.sm) {
                voteMark(vote)
                VStack(alignment: .leading, spacing: 0) {
                    Text(vote.headline)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(MonacoTheme.primaryText)
                        .lineLimit(dynamicTypeSize >= .xxxLarge ? 2 : 1)
                    Text(vote.line)
                        .font(.footnote)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .lineLimit(2)
                }
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .padding(.leading, MonacoTheme.Space.sm)
            .frame(minHeight: ChatPinMetrics.voteHeight)
            .contentShape(Rectangle().inset(by: -2))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(vote.spoken)
        .accessibilityAction(named: ChatPinCopy.goToVote) { goToVote() }
        .accessibilityAction(named: ChatPinCopy.showAll) { showAll() }
        .modifier(ChatPinVoteAction(enabled: vote.showsVote, action: castVote))
        .accessibilityIdentifier("chat-pin-vote-body")
    }

    @ViewBuilder private func voteMark(_ vote: ChatPinVote) -> some View {
        Group {
            if vote.isRule {
                SunkenGlyphMark(icon: .thesis, size: ChatPinMetrics.voteMark)
            } else {
                StockMark(
                    symbol: vote.symbol, displayName: vote.assetName, size: ChatPinMetrics.voteMark,
                    logoURL: vote.logoURL)
            }
        }
        .accessibilityHidden(true)
    }

    private var voteButton: some View {
        Button(action: castVote) {
            Text(ChatPinCopy.vote)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(MonacoTheme.primaryText)
                .padding(.horizontal, MonacoTheme.Space.sm)
                .frame(minHeight: ChatPinMetrics.voteButtonHeight)
                .background(Capsule().fill(MonacoTheme.surfaceSunken))
                .padding(.vertical, (ChatPinMetrics.minTarget - ChatPinMetrics.voteButtonHeight) / 2)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(ChatPinCopy.vote)
        .accessibilityIdentifier("chat-pin-vote-button")
    }

    private func nextButton(pinner: String, text: String, counter: String?) -> some View {
        Button(action: next) {
            HStack(spacing: MonacoTheme.Space.s) {
                Image(systemName: "pin")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                (Text(pinner).font(.footnote.weight(.semibold)) + Text(" ") + Text(text).font(.footnote))
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                    .truncationMode(.tail)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if let counter {
                    Text(counter)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize()
                }
            }
            .padding(.leading, MonacoTheme.Space.m)
            .frame(minHeight: ChatPinMetrics.height)
            .contentShape(Rectangle().inset(by: -2))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(ChatPinCopy.spokenLabel(pinner: pinner, text: text, counter: counter))
        .accessibilityHint(pins.pins.count > 1 ? ChatPinCopy.nextPin : "")
        .accessibilityAction(named: ChatPinCopy.nextPin) { next() }
        .accessibilityAction(named: ChatPinCopy.showAll) { showAll() }
        .accessibilityIdentifier("chat-pin-next")
    }

    private func listButton(height: CGFloat) -> some View {
        Button(action: showAll) {
            Image(systemName: "chevron.down")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minWidth: ChatPinMetrics.minTarget, minHeight: height)
                .contentShape(Rectangle().inset(by: -2))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(ChatPinCopy.showAll)
        .accessibilityIdentifier("chat-pin-list-button")
    }
}

private struct ChatPinVoteAction: ViewModifier {
    let enabled: Bool
    let action: () -> Void

    @ViewBuilder func body(content: Content) -> some View {
        if enabled {
            content.accessibilityAction(named: ChatPinCopy.vote, action)
        } else {
            content
        }
    }
}

private struct ChatPinCapsuleLayout: ViewModifier {
    let slivers: Bool

    func body(content: Content) -> some View {
        content
            .background(alignment: .top) { if slivers { ChatPinSlivers() } }
            .padding(.horizontal, ChatPinMetrics.sideMargin)
            .padding(.bottom, slivers ? (ChatPinMetrics.sliverDrops.max() ?? 0) : 0)
    }
}

private struct ChatPinSlivers: View {
    var body: some View {
        ZStack {
            sliver(index: 1, opacity: 0.4)
            sliver(index: 0, opacity: 0.7)
        }
        .allowsHitTesting(false)
        .accessibilityElement(children: .ignore)
        .accessibilityIdentifier("chat-pin-slivers")
    }

    private func sliver(index: Int, opacity: Double) -> some View {
        Capsule()
            .fill(Color.clear)
            .modifier(ChatPinCapsuleSurface())
            .opacity(opacity)
            .padding(.horizontal, ChatPinMetrics.sliverInsets[index])
            .offset(y: ChatPinMetrics.sliverDrops[index])
    }
}

private struct ChatPinCapsuleSurface: ViewModifier {
    @MonacoReduceTransparency private var reduceTransparency
    @Environment(\.displayScale) private var displayScale

    @ViewBuilder func body(content: Content) -> some View {
        if reduceTransparency {
            outlined(content.background(Capsule().fill(MonacoTheme.surfaceSunken)))
        } else if #available(iOS 26, *) {
            content.glassEffect(.regular.interactive(), in: .capsule)
        } else {
            outlined(content.background(Capsule().fill(.regularMaterial)))
        }
    }

    private func outlined(_ content: some View) -> some View {
        content.overlay(Capsule().strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale))
    }
}

struct ChatPinHost: ViewModifier {
    let pins: ChatPinsModel
    let cabalName: String
    let viewerID: String
    let controls: ChatPinControls
    let isShown: Bool
    var openVotes: ChatOpenVotesModel?
    var visibleCards: Set<String> = []
    var proposals: ChatProposalStore?
    var goToVote: (String) -> Void = { _ in }
    let scrollTo: (String) -> Void

    @State private var showsList = Self.opensListOnLaunch
    @State private var cards = CabalVoteCards()
    @Environment(ToastCenter.self) private var toasts
    @Environment(AppEnvironment.self) private var environment

    private var lead: ProposalSummary? { openVotes?.lead(hiding: visibleCards) }

    private var itemCount: Int { (openVotes?.count ?? 0) + pins.pins.count }

    private static var opensListOnLaunch: Bool {
        #if DEBUG
        ProcessInfo.processInfo.arguments.contains("-MonacoChatSamplePinsSheet")
        #else
        false
        #endif
    }

    private var hasItems: Bool { itemCount > 0 }

    private var visible: Bool { isShown && (lead != nil || !pins.pins.isEmpty) }

    func body(content: Content) -> some View {
        content
            .modifier(ChatPinSoftEdge(visible: visible, capsule: capsule))
            .environment(\.chatPinControls, controls)
            .sensoryFeedback(.selection, trigger: pins.current)
            .onChange(of: pins.toast) { _, toast in
                if let toast { toasts.current = MonacoToast(message: toast.message) }
            }
            .onChange(of: hasItems) { _, has in
                if !has { showsList = false }
            }
            .sheet(isPresented: $showsList) {
                ChatPinListSheet(
                    pins: pins, cabalName: cabalName, viewerID: viewerID, controls: controls, openVotes: openVotes,
                    proposals: proposals,
                    goToVote: { id in
                        showsList = false
                        goToVote(id)
                    }
                ) { id in
                    showsList = false
                    scrollTo(id)
                }
            }
    }

    private var capsule: some View {
        TimelineView(.everyMinute) { _ in
            let vote = lead.map { pinVote($0, now: Date()) }
            ChatPinCapsule(
                pins: pins, vote: vote, itemCount: itemCount, cabalName: cabalName, viewerID: viewerID,
                next: {
                    pins.next()
                    if pins.pins.indices.contains(pins.current) { scrollTo(pins.pins[pins.current].id) }
                },
                showAll: { showsList = true },
                castVote: {
                    if let vote { environment.navigator.presentVoteHold(proposalID: vote.id, pick: nil) }
                },
                goToVote: {
                    if let vote { goToVote(vote.id) }
                })
        }
    }

    private func pinVote(_ summary: ProposalSummary, now: Date) -> ChatPinVote {
        var asset: ProposalAsset?
        if case .loaded(let facts) = proposals?.state(for: summary.id) { asset = facts.asset }
        let row = CabalProposals.pendingVote(summary, asset: asset)
        let card = cards.card(for: row, ballot: nil, environment: environment) { _, _ in }
        return ChatPinVote(summary: summary, headline: card.headline, asset: asset, now: now)
    }
}

private struct ChatPinSoftEdge<Capsule: View>: ViewModifier {
    let visible: Bool
    let capsule: Capsule

    @ViewBuilder func body(content: Content) -> some View {
        if #available(iOS 26, *) {
            content
                .scrollEdgeEffectStyle(visible ? .soft : .automatic, for: .top)
                .safeAreaBar(edge: .top, spacing: 0) { if visible { capsule } }
        } else {
            content.overlay(alignment: .top) { if visible { capsule } }
        }
    }
}
