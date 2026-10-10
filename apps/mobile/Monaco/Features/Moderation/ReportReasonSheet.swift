import MonacoAPI
import MonacoCore
import SwiftUI

struct ReportTarget: Identifiable, Equatable {
    let kind: ReportKind
    let targetID: String

    var id: String { "\(kind.rawValue)-\(targetID)" }
}

struct ReportReasonSheet: View {
    let target: ReportTarget

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var rootToasts
    @Environment(\.dismiss) private var dismiss
    @State private var model: Flow20aReportBlockModel?
    @State private var toasts = ToastCenter()
    @State private var isSending = false

    var body: some View {
        VStack(spacing: 0) {
            ForEach(ReportReason.allCases) { reason in
                Button {
                    Task { await send(reason) }
                } label: {
                    Text(reason.label)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.ink)
                        .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .disabled(isSending)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("report-reason-\(reason.rawValue)")
                MonacoRule().padding(.leading, MonacoTheme.Space.gutter)
            }
            Button("Cancel") { dismiss() }
                .buttonStyle(.monacoSecondary)
                .padding(MonacoTheme.Space.gutter)
                .accessibilityIdentifier("report-cancel")
        }
        .frame(maxHeight: .infinity, alignment: .top)
        .monacoSheet(title: "Report", titleIdentifier: "report-title")
        .monacoToastCenter(toasts)
        .presentationDetents([.medium])
    }

    private func send(_ reason: ReportReason) async {
        guard !isSending else { return }
        isSending = true
        defer { isSending = false }
        let actions =
            model ?? Flow20aReportBlockModel(api: environment.api, hints: environment.hints)
        model = actions
        let outcome = await actions.report(kind: target.kind, targetID: target.targetID, reason: reason)
        let message = actions.toast(for: outcome, after: .report)
        if outcome == .ok {
            rootToasts.show(success: message)
            dismiss()
        } else {
            toasts.current = MonacoToast(message: message)
        }
    }
}

extension View {
    func reportSheet(_ target: Binding<ReportTarget?>) -> some View {
        sheet(item: target) { ReportReasonSheet(target: $0) }
    }
}
