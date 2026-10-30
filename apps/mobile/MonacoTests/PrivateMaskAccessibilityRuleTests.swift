import Testing

struct PrivateMaskAccessibilityRuleTests {
    @Test
    func flagsAMaskedTextThatHidesItsIdentifier() {
        let source = """
            Text(text)
                .multilineTextAlignment(.center)
                .monacoPrivate()
                .accessibilityIdentifier("amount-keypad-line")
            """
        #expect(PrivateMaskAccessibilityRule.violations(in: source) == [3])
    }

    @Test
    func passesTheFigureModifier() {
        let source = """
            Text(text)
                .monacoPrivateFigure()
                .accessibilityIdentifier("amount-keypad-line")
            """
        #expect(PrivateMaskAccessibilityRule.violations(in: source).isEmpty)
    }

    @Test
    func passesAMaskFollowedByACombinedElement() {
        let source = """
            Text(text)
                .monacoPrivate()
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("amount-keypad-line")
            """
        #expect(PrivateMaskAccessibilityRule.violations(in: source).isEmpty)
    }

    @Test
    func passesTheAllowlistedTextField() {
        let source = """
            TextField("", text: $code)
                .monacoPrivate()
                .accessibilityIdentifier("referral-code-field")
            """
        #expect(PrivateMaskAccessibilityRule.violations(in: source).isEmpty)
    }
}
