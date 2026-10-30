package model

import "net/http"

// CodedError is a business condition the API answers with its own HTTP status
// and error code (localized under billing.errors.<Code>). It unwraps to the
// generic sentinel, so errors.Is(err, ErrConflict) keeps working (CF-154).
type CodedError struct {
	Status int
	Code   string
	Base   error
}

func (e *CodedError) Error() string { return "billing: " + e.Code }
func (e *CodedError) Unwrap() error { return e.Base }

var (
	// ErrSellerNotConfigured: BILLING_SELLER_JSON is empty or invalid and the
	// organization has no earlier invoice to copy the seller from. Before
	// CF-154 this surfaced as a 503 "payment gateway not configured".
	ErrSellerNotConfigured = &CodedError{Status: http.StatusConflict, Code: "BILLING_SELLER_NOT_CONFIGURED", Base: ErrConflict}
	// ErrProfileIncomplete: the organization's billing profile is missing or
	// fails validation (e-Arşiv needs it on every invoice).
	ErrProfileIncomplete = &CodedError{Status: http.StatusUnprocessableEntity, Code: "BILLING_PROFILE_INCOMPLETE", Base: ErrInvalid}
	// ErrAccountMissing: the organization has no subscription to bill.
	ErrAccountMissing = &CodedError{Status: http.StatusConflict, Code: "BILLING_ACCOUNT_MISSING", Base: ErrConflict}
	// ErrPackageUnavailable: the credit package does not exist, is inactive or
	// is not offered on the organization's plan.
	ErrPackageUnavailable = &CodedError{Status: http.StatusNotFound, Code: "CREDIT_PACKAGE_UNAVAILABLE", Base: ErrNotFound}
	// ErrNoPaymentMethod: neither card checkout nor bank transfer can pay an
	// invoice right now, so none is opened.
	ErrNoPaymentMethod = &CodedError{Status: http.StatusConflict, Code: "PAYMENT_METHOD_UNAVAILABLE", Base: ErrConflict}
	// ErrCheckoutInProgress: another charge of the invoice is still being
	// settled (an automatic renewal charge or an unresolved provider call).
	ErrCheckoutInProgress = &CodedError{Status: http.StatusConflict, Code: "CHECKOUT_IN_PROGRESS", Base: ErrConflict}
	// ErrInvoiceNotPayable: the invoice is paid, void or a credit note.
	ErrInvoiceNotPayable = &CodedError{Status: http.StatusConflict, Code: "INVOICE_NOT_PAYABLE", Base: ErrConflict}
)

// ErrEInvoiceNotConfigured: no e-invoice integrator is set
// (BILLING_EINVOICE_PROVIDER), so the document cannot be issued yet.
var ErrEInvoiceNotConfigured = &CodedError{Status: http.StatusConflict, Code: "EINVOICE_NOT_CONFIGURED", Base: ErrConflict}

// ErrCheckoutFinished: the idempotency key belongs to a checkout that already
// failed or expired; a new attempt needs a new key.
var ErrCheckoutFinished = &CodedError{Status: http.StatusConflict, Code: "CHECKOUT_FINISHED", Base: ErrConflict}
