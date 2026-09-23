package handlers

import "testing"

func TestReceiptMessageTemplateAllowlist(t *testing.T) {
	for _, value := range []string{
		"Hi {{customer_name}}, invoice {{invoice_number}} total {{order_total}}",
		"{{paid_amount}} {{outstanding_amount}} {{due_date}} {{order_status}}",
	} {
		if err := validateReceiptMessageTemplate(value); err != nil {
			t.Fatalf("valid template rejected: %v", err)
		}
	}
	for _, value := range []string{"{{unknown}}", "{{customer_name", "customer_name}}", "{{}}"} {
		if err := validateReceiptMessageTemplate(value); err == nil {
			t.Errorf("invalid template %q accepted", value)
		}
	}
}

func TestRenderReceiptMessageUsesPlainTextValues(t *testing.T) {
	got := renderReceiptMessage("{{customer_name}} owes {{outstanding_amount}}", map[string]string{
		"customer_name": "A & <B>", "outstanding_amount": rupiah(1234567),
	})
	if want := "A & <B> owes Rp 1.234.567"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReceiptTemplateInputRejectsUnsupportedPaperWidth(t *testing.T) {
	name, width := "Default", int32(72)
	input := receiptTemplateInput{Name: &name, PaperWidthMM: &width}
	if err := input.validate(false); err == nil {
		t.Fatal("unsupported printer preference was accepted")
	}
}
