import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalMoreItem: CaseIterable {
    case chat
    case edit
    case treasury
    case report
    case leave

    var title: String {
        switch self {
        case .chat: "Chat"
        case .edit: "Edit name and picture"
        case .treasury: "Treasury address"
        case .report: "Report cabal"
        case .leave: "Leave cabal"
        }
    }

    var systemImage: String {
        switch self {
        case .chat: MonacoIcon.chat.symbol
        case .edit: MonacoIcon.edit.symbol
        case .treasury: MonacoIcon.wallet.symbol
        case .report: MonacoIcon.report.symbol
        case .leave: MonacoIcon.trash.symbol
        }
    }

    var identifier: String {
        switch self {
        case .chat: "cabal-more-chat"
        case .edit: "cabal-more-edit"
        case .treasury: "cabal-more-treasury"
        case .report: "cabal-more-report"
        case .leave: "cabal-more-leave"
        }
    }

    static func items(role: String?, status: String) -> [CabalMoreItem] {
        guard role != nil else { return [.report] }
        var items: [CabalMoreItem] = []
        if status != CabalClosedModel.closedStatus { items.append(.chat) }
        if role == "creator" { items.append(.edit) }
        items.append(.treasury)
        if role != "creator" { items.append(.report) }
        items.append(.leave)
        return items
    }
}

struct CabalMoreMenu: View {
    let items: [CabalMoreItem]
    let select: (CabalMoreItem) -> Void

    var body: some View {
        Menu {
            ForEach(items, id: \.self) { item in
                Button(role: item == .leave ? .destructive : nil) {
                    select(item)
                } label: {
                    Label(item.title, systemImage: item.systemImage)
                }
                .accessibilityIdentifier(item.identifier)
            }
        } label: {
            Image(systemName: "ellipsis")
                .monacoToolbarIcon()
                .frame(minWidth: 44, minHeight: 44)
        }
        .accessibilityLabel("More")
        .accessibilityIdentifier("cabal-more-button")
    }
}

private enum CabalMoreSheet: String, Identifiable {
    case treasury
    case leave

    var id: String { rawValue }
}

struct CabalMoreActions: ViewModifier {
    let cabalID: String
    let cabal: CabalModel
    let edit: CabalEditModel
    @Binding var pending: CabalMoreItem?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.hostMainTab) private var hostMainTab
    @State private var sheet: CabalMoreSheet?
    @State private var editing = false
    @State private var reporting: ReportTarget?

    func body(content: Content) -> some View {
        content
            .onChange(of: pending) { _, item in
                guard let item else { return }
                pending = nil
                run(item)
            }
            .reportSheet($reporting)
            .navigationDestination(isPresented: $editing) {
                if let loaded = edit.cabal {
                    EditCabalView(
                        model: edit, cabal: loaded, pictureWriter: LiveCabalPictureWriter(api: environment.api))
                }
            }
            .sheet(item: $sheet) { sheet in
                switch sheet {
                case .treasury: treasury
                case .leave: leave
                }
            }
    }

    private func run(_ item: CabalMoreItem) {
        switch item {
        case .chat:
            environment.navigator.open(
                ChatRoute(cabalID: cabalID), in: hostMainTab ?? environment.navigator.selectedTab)
        case .edit:
            Task {
                await edit.load()
                if edit.cabal != nil {
                    editing = true
                } else if let error = edit.lastError {
                    toasts.show(error)
                }
            }
        case .treasury: sheet = .treasury
        case .report: reporting = ReportTarget(kind: .cabal, targetID: cabalID)
        case .leave: sheet = .leave
        }
    }

    private var treasury: some View {
        CabalTreasuryView(model: cabal, onReloadFailure: { toasts.show($0) })
            .padding(.bottom, MonacoTheme.Space.m)
            .monacoFittedSheet(title: CabalTreasurySlotCopy.header, showsClose: true)
            .monacoToastCenter(toasts)
    }

    private var leave: some View {
        CabalLeaveSection(cabalID: cabalID)
            .padding(.bottom, MonacoTheme.Space.m)
            .monacoFittedSheet(title: CabalMoreItem.leave.title, showsClose: true)
            .monacoToastCenter(toasts)
    }
}

extension View {
    func cabalMoreActions(
        cabalID: String, cabal: CabalModel, edit: CabalEditModel, pending: Binding<CabalMoreItem?>
    ) -> some View {
        modifier(CabalMoreActions(cabalID: cabalID, cabal: cabal, edit: edit, pending: pending))
    }
}
