import MonacoCore
import SwiftUI

struct ThesisQuoteSheet: View {
    let thesis: String
    let onPick: (CommentAttachmentDraft) -> Void

    @State private var selection: TextSelection?
    @Environment(\.dismiss) private var dismiss

    private var selectedRange: Range<String.Index>? {
        guard let selection else { return nil }
        switch selection.indices {
        case .selection(let range): return range.isEmpty ? nil : range
        case .multiSelection(let set): return set.ranges.first
        @unknown default: return nil
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack {
                Text("Quote the thesis")
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityAddTraits(.isHeader)
                Spacer(minLength: 0)
                MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", style: .quiet, size: .compact) {
                    dismiss()
                }
                .accessibilityIdentifier("thesis-sheet-close")
            }
            .frame(minHeight: 48)
            TextEditor(text: Binding(get: { thesis }, set: { _ in }), selection: $selection)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.primaryText)
                .scrollContentBackground(.hidden)
                .accessibilityIdentifier("thesis-sheet-text")
            HStack(spacing: MonacoTheme.Space.s) {
                Button("Quote selection") {
                    guard let range = selectedRange else { return }
                    pick(ThesisQuote.selection(range, in: thesis))
                }
                .buttonStyle(.monacoSecondary)
                .disabled(selectedRange == nil)
                .accessibilityIdentifier("thesis-quote-selection")
                Button("Quote all") { pick(ThesisQuote.all(thesis)) }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("thesis-quote-all")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(MonacoTheme.canvas.ignoresSafeArea())
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("thesis-sheet")
    }

    private func pick(_ draft: CommentAttachmentDraft) {
        onPick(draft)
        dismiss()
    }
}
