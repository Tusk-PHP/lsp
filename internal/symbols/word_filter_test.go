package symbols

import "testing"

func TestWordFilterHoldsEveryWordIgnoringCase(t *testing.T) {
	f := newWordFilter("<?php\nclass PaymentGateway { public function charge_card($amount) {} }\n// see Refund2\nfinal")
	for _, word := range []string{"PaymentGateway", "paymentgateway", "charge_card", "amount", "Refund2", "final", "php"} {
		if !f.mayContain(word) {
			t.Errorf("%s is in the source", word)
		}
	}
	for _, word := range []string{"Payment", "Gateway", "refund", "Checkout"} {
		if f.mayContain(word) {
			t.Errorf("%s is not a word of the source", word)
		}
	}
}
