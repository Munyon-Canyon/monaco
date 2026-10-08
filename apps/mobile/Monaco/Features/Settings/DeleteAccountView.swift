import MonacoAPI
import MonacoCore
import SwiftUI

struct DeleteAccountView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: DeleteAccountModel?

    var body: some View {
        DeleteAccountContent(model: model, delete: delete)
            .onAppear {
                let model = preparedModel()
                Task { await model.load() }
            }
            .onChange(of: model?.failureTick) { _, _ in
                guard let message = model?.toastMessage else { return }
                toasts.current = MonacoToast(message: message)
            }
    }

    private func preparedModel() -> DeleteAccountModel {
        if let model { return model }
        let created = DeleteAccountModel(api: environment.api)
        model = created
        return created
    }

    private func delete() async {
        let model = preparedModel()
        await model.delete()
        guard model.isDeleted else { return }
        await environment.signOut()
        toasts.show(success: AccountCopy.deleted)
    }
}

struct DeleteAccountContent: View {
    let model: DeleteAccountModel?
    let delete: () async -> Void

    @State private var confirming = false

    private var isDeleting: Bool { model?.isDeleting ?? false }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                Text(AccountCopy.explainer)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, MonacoTheme.Space.m)
                    .accessibilityIdentifier("delete-account-explainer")
                checklist
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button {
                    confirming = true
                } label: {
                    HStack(spacing: MonacoTheme.Space.s) {
                        if isDeleting { ProgressView() }
                        Text(isDeleting ? AccountCopy.deleting : AccountCopy.deleteTitle)
                    }
                }
                .buttonStyle(.monacoDestructive)
                .disabled(isDeleting)
                .accessibilityIdentifier("delete-account-button")
                .confirmationDialog(AccountCopy.confirmTitle, isPresented: $confirming, titleVisibility: .visible) {
                    Button(AccountCopy.confirmDelete, role: .destructive) {
                        Task { await delete() }
                    }
                    .accessibilityIdentifier("delete-account-confirm")
                    Button(AccountCopy.cancel, role: .cancel) {}
                        .accessibilityIdentifier("delete-account-cancel")
                }
            }
        }
        .navigationTitle(AccountCopy.deleteTitle)
        .navigationBarTitleDisplayMode(.inline)
    }

    @ViewBuilder private var checklist: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 2, markShape: .tile)
                .accessibilityIdentifier("delete-account-loading")
        case .failed:
            EmptyState(title: AccountCopy.loadFailed, actionTitle: AccountCopy.tryAgain) {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("delete-account-failed")
        case .loaded(let checklist):
            cashOutStep(checklist)
            withdrawStep(checklist)
        }
    }

    private func cashOutStep(_ checklist: DeleteAccountChecklist) -> some View {
        ChecklistStep(
            title: AccountCopy.cashOutStep,
            isDone: checklist.isCashedOut,
            isHighlighted: model?.highlighted == .cashOutFirst
        ) {
            if checklist.isCashedOut {
                DoneLine(detail: AccountCopy.noCabalMoney)
            } else {
                MonacoGroupedList {
                    ForEach(Array(checklist.slices.enumerated()), id: \.element.id) { index, slice in
                        NavigationLink(value: AnyAppRoute(CashOutRoute(cabalID: slice.cabal.id))) {
                            MonacoRow(
                                title: slice.cabal.name,
                                subtitle: AccountCopy.yourSlice(UsdAmountFormatter.format(micros: slice.valueMicros)),
                                chevron: true, isLast: index == checklist.slices.count - 1
                            ) {
                                CabalMark(
                                    groupId: slice.cabal.id, name: slice.cabal.name, size: 40,
                                    pictureUrl: slice.cabal.pictureUrl)
                            }
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("delete-account-cabal-\(slice.cabal.id)")
                    }
                }
            }
        }
        .accessibilityIdentifier("delete-account-step-cash-out")
    }

    private func withdrawStep(_ checklist: DeleteAccountChecklist) -> some View {
        let amount = UsdAmountFormatter.format(micros: checklist.balance.availableMicros)
        return ChecklistStep(
            title: AccountCopy.withdrawStep,
            isDone: checklist.isWithdrawn,
            isHighlighted: model?.highlighted == .withdrawFirst
        ) {
            if checklist.isWithdrawn {
                DoneLine(detail: amount)
            } else {
                MonacoGroupedList {
                    NavigationLink(value: AnyAppRoute(WithdrawRoute())) {
                        MonacoRow(
                            title: AccountCopy.accountBalance, chevron: true, isLast: true,
                            leading: { StockMark(systemImage: "arrow.down.left") },
                            trailing: {
                                Text(amount)
                                    .font(MonacoTheme.Typo.moneyRow)
                                    .foregroundStyle(MonacoTheme.ink)
                            }
                        )
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("delete-account-withdraw")
                }
            }
        }
        .accessibilityIdentifier("delete-account-step-withdraw")
    }
}

private struct ChecklistStep<Content: View>: View {
    let title: String
    let isDone: Bool
    let isHighlighted: Bool
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: MonacoTheme.Space.s) {
                Image(systemName: isDone ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(isDone ? MonacoTheme.success : stepColor)
                    .accessibilityHidden(true)
                Text(title)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(stepColor)
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            .accessibilityElement(children: .combine)
            .accessibilityAddTraits(.isHeader)
            content
        }
        .padding(.vertical, isHighlighted ? MonacoTheme.Space.s : 0)
        .background {
            if isHighlighted {
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                    .fill(MonacoTheme.lossWash)
                    .padding(.horizontal, MonacoTheme.Space.xs)
            }
        }
        .accessibilityElement(children: .contain)
    }

    private var stepColor: Color { isHighlighted ? MonacoTheme.destructive : MonacoTheme.ink }
}

private struct DoneLine: View {
    let detail: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
            Text(AccountCopy.done)
                .font(MonacoTheme.Typo.bodyStrong)
                .foregroundStyle(MonacoTheme.success)
            Text(detail)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .accessibilityElement(children: .combine)
    }
}
