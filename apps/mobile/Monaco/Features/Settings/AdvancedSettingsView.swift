import SwiftUI

struct AdvancedSettingsView: View {
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                MonacoGroupedList {
                    ForEach(SettingsAdvancedLinks.explorerLinks) { link in
                        Link(destination: link.url) {
                            MonacoRow(
                                title: link.title,
                                subtitle: link.url.host() ?? link.url.absoluteString,
                                isLast: link.id == SettingsAdvancedLinks.explorerLinks.last?.id,
                                leading: { SunkenGlyphMark(systemImage: "safari") },
                                trailing: {
                                    Image(systemName: "arrow.up.right")
                                        .font(MonacoTheme.Typo.captionStrong)
                                        .foregroundStyle(MonacoTheme.muted)
                                        .accessibilityHidden(true)
                                }
                            )
                        }
                        .buttonStyle(.monacoRow)
                        .accessibilityIdentifier("settings-explorer-\(link.id)")
                    }
                }
                MonacoNotice(.info, message: "Opens in Safari. Monaco never asks you to sign anything there.")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.s)
                    .accessibilityIdentifier("settings-explorers-notice")
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Block explorers")
        .navigationBarTitleDisplayMode(.inline)
    }
}

#Preview {
    NavigationStack {
        AdvancedSettingsView()
    }
}
