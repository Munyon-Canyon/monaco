import SwiftUI
import UIKit

struct MonacoFieldChrome: ViewModifier {
    let isFocused: Bool
    var isInvalid = false
    var isOutlined = false
    var minHeight = MonacoFieldChrome.height
    let focus: () -> Void

    static let height: CGFloat = 48
    static let multilineHeight: CGFloat = 92
    static let borderWidth: CGFloat = 1

    private var shape: RoundedRectangle {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
    }

    private var stroke: Color {
        if isInvalid { return MonacoTheme.loss }
        return isFocused ? MonacoTheme.brand : MonacoTheme.border
    }

    func body(content: Content) -> some View {
        content
            .padding(.horizontal, MonacoTheme.Space.m)
            .padding(.vertical, MonacoTheme.Space.sm)
            .frame(minHeight: minHeight)
            .background(shape.fill(MonacoTheme.surface))
            .contentShape(Rectangle())
            .simultaneousGesture(TapGesture().onEnded { focus() })
            .overlay {
                shape.strokeBorder(stroke, lineWidth: Self.borderWidth)
            }
            .animation(.easeOut(duration: 0.15), value: isFocused)
            .animation(.easeOut(duration: 0.15), value: isInvalid)
    }
}

extension View {
    /// Draws this text field as every Monaco field is drawn. See `MonacoFieldChrome`.
    func monacoFieldChrome(
        isFocused: Bool,
        isInvalid: Bool = false,
        isOutlined: Bool = false,
        minHeight: CGFloat = MonacoFieldChrome.height,
        focus: @escaping () -> Void
    ) -> some View {
        modifier(
            MonacoFieldChrome(
                isFocused: isFocused, isInvalid: isInvalid, isOutlined: isOutlined, minHeight: minHeight, focus: focus))
    }
}

private struct MonacoFieldFrame<Field: View>: View {
    let label: String?
    let helper: String?
    let error: String?
    @ViewBuilder let field: Field

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let label {
                Text(label)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .padding(.bottom, MonacoTheme.Space.s)
                    .accessibilityHidden(true)
            }
            field
            if let error, !error.isEmpty {
                Text(error)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.loss)
                    .padding(.top, MonacoTheme.Space.xs)
                    .accessibilityHidden(true)
            } else if let helper {
                Text(helper)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .padding(.top, MonacoTheme.Space.s)
            }
        }
    }
}

struct MonacoTextField: View {
    private let placeholder: String
    @Binding private var text: String
    private let keyboard: UIKeyboardType
    private let contentType: UITextContentType?
    private let isInvalid: Bool
    private let isOutlined: Bool
    private let errorMessage: String?
    private let autofocus: Bool
    private let capitalization: TextInputAutocapitalization?
    private let autocorrects: Bool?
    private let label: String?
    private let helper: String?
    private let isMultiline: Bool

    @FocusState private var focused: Bool
    @Environment(\.isEnabled) private var isEnabled

    init(
        _ placeholder: String,
        text: Binding<String>,
        keyboard: UIKeyboardType = .default,
        contentType: UITextContentType? = nil,
        isInvalid: Bool = false,
        isOutlined: Bool = false,
        errorMessage: String? = nil,
        autofocus: Bool = false,
        capitalization: TextInputAutocapitalization? = nil,
        autocorrects: Bool? = nil,
        label: String? = nil,
        helper: String? = nil,
        isMultiline: Bool = false
    ) {
        self.placeholder = placeholder
        _text = text
        self.keyboard = keyboard
        self.contentType = contentType
        self.isInvalid = isInvalid
        self.isOutlined = isOutlined
        self.errorMessage = errorMessage
        self.autofocus = autofocus
        self.capitalization = capitalization
        self.autocorrects = autocorrects
        self.label = label
        self.helper = helper
        self.isMultiline = isMultiline
    }

    var body: some View {
        MonacoFieldFrame(label: label, helper: helper, error: isInvalid ? errorMessage : nil) {
            input
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .tint(MonacoTheme.ink)
                .keyboardType(keyboard)
                .textContentType(contentType)
                .textInputAutocapitalization(autocapitalization)
                .autocorrectionDisabled(disablesAutocorrection)
                .focused($focused)
                .task {
                    if autofocus { focused = true }
                }
                .monacoFieldChrome(
                    isFocused: focused, isInvalid: isInvalid, isOutlined: isOutlined,
                    minHeight: isMultiline ? MonacoFieldChrome.multilineHeight : MonacoFieldChrome.height,
                    focus: { focused = true }
                )
                .opacity(isOutlined && !isEnabled ? 0.5 : 1)
                .accessibilityLabel(label ?? placeholder)
                .accessibilityValue(isInvalid ? (errorMessage ?? "") : "")
                .monacoPrivate()
        }
    }

    @ViewBuilder
    private var input: some View {
        if isMultiline {
            TextField("", text: $text, prompt: Text(placeholder).foregroundStyle(placeholderColor), axis: .vertical)
                .lineLimit(4...8)
        } else {
            TextField("", text: $text, prompt: Text(placeholder).foregroundStyle(placeholderColor))
        }
    }

    private var placeholderColor: Color {
        isOutlined ? MonacoTheme.secondaryText : MonacoTheme.disabledLabel
    }

    private var isCodeOrContact: Bool {
        switch keyboard {
        case .emailAddress, .numberPad, .phonePad, .URL, .asciiCapableNumberPad, .decimalPad: return true
        default: return contentType == .oneTimeCode || contentType == .emailAddress
        }
    }

    private var autocapitalization: TextInputAutocapitalization {
        capitalization ?? (isCodeOrContact ? .never : .sentences)
    }

    private var disablesAutocorrection: Bool {
        autocorrects.map { !$0 } ?? isCodeOrContact
    }
}

/// A field for a Solana address: monospace, wraps by character, never hyphenates, takes no
/// newline. `TextField(axis: .vertical)` hyphenates a long word where it wraps, and an address
/// must never carry a hyphen (see `MonacoWalletAddressText`), so this is a `UITextView` told to
/// break lines by character. Drawn with `MonacoFieldChrome` like every other field.
struct MonacoAddressField: View {
    let placeholder: String
    @Binding var text: String
    var isInvalid = false
    var label: String?
    var helper: String?
    var errorMessage: String?
    /// Set on the text view itself: a modifier on the wrapper does not reach the element the UI
    /// tests type into.
    var accessibilityIdentifier: String?

    @State private var isFocused = false
    @State private var focusRequests = 0

    var body: some View {
        MonacoFieldFrame(label: label, helper: helper, error: isInvalid ? errorMessage : nil) {
            ZStack(alignment: .leading) {
                if text.isEmpty {
                    Text(placeholder)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.disabledLabel)
                        .accessibilityHidden(true)
                }
                CharacterWrappingTextView(
                    text: $text,
                    isFocused: $isFocused,
                    focusRequests: focusRequests,
                    placeholder: placeholder,
                    accessibilityIdentifier: accessibilityIdentifier
                )
            }
            .monacoFieldChrome(isFocused: isFocused, isInvalid: isInvalid, focus: { focusRequests += 1 })
            .monacoPrivate()
        }
    }
}

private struct CharacterWrappingTextView: UIViewRepresentable {
    @Binding var text: String
    @Binding var isFocused: Bool
    let focusRequests: Int
    let placeholder: String
    let accessibilityIdentifier: String?

    /// `Typo.data`: 15pt mono, medium, scaled with the subheadline style.
    private static var font: UIFont {
        UIFontMetrics(forTextStyle: .subheadline)
            .scaledFont(for: .monospacedSystemFont(ofSize: 15, weight: .medium))
    }

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeUIView(context: Context) -> UITextView {
        let view = UITextView()
        view.delegate = context.coordinator
        view.backgroundColor = .clear
        view.isScrollEnabled = false
        view.textContainerInset = .zero
        view.textContainer.lineFragmentPadding = 0
        view.textContainer.lineBreakMode = .byCharWrapping
        view.keyboardType = .asciiCapable
        view.returnKeyType = .done
        view.autocorrectionType = .no
        view.autocapitalizationType = .none
        view.spellCheckingType = .no
        // A smart dash would turn a typed hyphen into an en dash inside an address.
        view.smartDashesType = .no
        view.smartQuotesType = .no
        view.smartInsertDeleteType = .no
        view.adjustsFontForContentSizeCategory = true
        view.font = Self.font
        view.textColor = UIColor(MonacoTheme.ink)
        view.tintColor = UIColor(MonacoTheme.ink)
        view.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        view.setContentHuggingPriority(.required, for: .vertical)
        view.accessibilityLabel = placeholder
        view.accessibilityIdentifier = accessibilityIdentifier
        return view
    }

    func updateUIView(_ view: UITextView, context: Context) {
        context.coordinator.parent = self
        if view.text != text {
            view.text = text
        }
        if focusRequests != context.coordinator.focusRequests {
            context.coordinator.focusRequests = focusRequests
            view.becomeFirstResponder()
        }
    }

    func sizeThatFits(_ proposal: ProposedViewSize, uiView: UITextView, context: Context) -> CGSize? {
        let proposed = proposal.width ?? uiView.bounds.width
        let width = proposed.isFinite && proposed > 0 ? proposed : 320
        let fitted = uiView.sizeThatFits(CGSize(width: width, height: .greatestFiniteMagnitude))
        return CGSize(width: width, height: fitted.height)
    }

    final class Coordinator: NSObject, UITextViewDelegate {
        var parent: CharacterWrappingTextView
        var focusRequests = 0

        init(_ parent: CharacterWrappingTextView) {
            self.parent = parent
        }

        func textViewDidChange(_ view: UITextView) {
            parent.text = view.text
        }

        func textViewDidBeginEditing(_ view: UITextView) {
            parent.isFocused = true
        }

        func textViewDidEndEditing(_ view: UITextView) {
            parent.isFocused = false
        }

        /// Return is Done: an address has no second line.
        func textView(_ view: UITextView, shouldChangeTextIn range: NSRange, replacementText text: String) -> Bool {
            guard text.contains("\n") else { return true }
            view.resignFirstResponder()
            return false
        }
    }
}
