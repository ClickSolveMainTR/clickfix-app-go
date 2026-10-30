// Package payments ports Takwim platform/payments: the Provider interface every online payment
// adapter implements on top of the integration skeleton (accounts with
// sealed credentials, inbound notification idempotency).
//
// Card data never touches Takwim: CreateCheckout opens the provider's
// hosted page and returns its address; the customer pays there. A browser
// return is never proof of a payment: only a provider notification whose
// signature VerifyNotification accepted with the business's own secret
// records a result.
package payments

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Notification statuses (provider statuses are mapped onto these).
const (
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// Buyer is the payer data a provider asks for (fraud checks). No card data.
type Buyer struct {
	Name  string
	Email string
	Phone string
	IP    string
}

// CheckoutRequest opens a hosted checkout for one full payment.
type CheckoutRequest struct {
	// OrderRef is our reference of the payment (letters and digits only);
	// the provider echoes it in its notification.
	OrderRef string
	Amount   int64
	Currency string
	// ItemName names what is paid ("Kesim · #12").
	ItemName string
	Buyer    Buyer
	// SuccessURL / FailURL are where the browser goes back; the page they
	// open shows the server-verified state, never "paid" on its own.
	SuccessURL string
	FailURL    string
	Locale     string
	// ExpiresAt bounds the provider page (the reservation's payment window).
	ExpiresAt time.Time
	// ReturnURL is where a provider that confirms through its API (a
	// Confirmer, iyzico) sends the browser back with its token: the
	// panel's /pay/return/{provider}/{ref}, which asks the API for the
	// result before it shows SuccessURL or FailURL.
	ReturnURL string
	// CardOwnerKey is the provider's key of the payer's saved cards (from
	// an earlier SavedCard), so the page lists them and keeps a newly
	// saved card under the same owner. Empty for a first payment.
	CardOwnerKey string
	// Recurring marks a payment that starts a renewing charge (a
	// membership): providers that know the difference are told.
	Recurring bool
	StoreCard bool
}

// Checkout is an opened hosted page.
type Checkout struct {
	PageURL string
	// Token is the provider's reference of the page (not a secret).
	Token string
}

// Notification is a parsed provider notification (not trusted before
// VerifyNotification).
type Notification struct {
	// OrderRef is our payment reference (CheckoutRequest.OrderRef).
	OrderRef string
	// EventKey identifies the notification at the provider: the same
	// notification delivered again has the same key.
	EventKey string
	Status   string
	// Amount is what the payment was for; PaidAmount what the customer
	// paid (installment interest included), both in Currency.
	Amount     int64
	PaidAmount int64
	Currency   string
	// ProviderPaymentID is the provider's payment reference when it has one.
	ProviderPaymentID string
	FailureCode       string
	FailureMessage    string
	Test              bool
	// Token is the provider's token of the page, when the notification
	// carries one (a Confirmer reads the result with it).
	Token string
	// Card is the card the payer chose to save on the provider's page
	// (nil when none was saved, or the provider does not store cards).
	Card *SavedCard
}

// Notification statuses only a Confirmer answers: the provider has the
// payment but not its final state yet (3-D Secure still open, a fraud
// review). Nothing is recorded; the result is read again later.
const StatusPending = "pending"

// SavedCard is a card the provider keeps for the business (card storage):
// the provider's keys to charge it again, never card data. Last4, Brand,
// Family and Bank are what the provider shows of it.
type SavedCard struct {
	// OwnerKey is the provider's key of the card's owner (iyzico
	// cardUserKey); one owner can have several cards.
	OwnerKey string
	// Token is the provider's key of the card (iyzico cardToken).
	Token string
	// Last4 and Bin are the visible digits ("" when the provider gives none).
	Last4 string
	Bin   string
	// Brand is the card scheme (VISA, MASTER_CARD, TROY, AMERICAN_EXPRESS).
	Brand string
	// Family is the card program (Bonus, World, Axess …) and Bank the issuer.
	Family string
	Bank   string
	// Alias is the name the payer gave the card on the page.
	Alias string
}

// Provider is one online payment provider.
type Provider interface {
	interface{ Code() string }
	// CreateCheckout opens the hosted payment page of a payment.
	CreateCheckout(ctx context.Context, account Account, in CheckoutRequest) (Checkout, error)
	// ParseNotification reads a notification body; nothing is trusted yet
	// (the business, hence the secret, is found from its OrderRef).
	ParseNotification(header http.Header, body []byte) (Notification, error)
	// VerifyNotification checks the notification's signature with the
	// account's secret.
	VerifyNotification(account Account, n Notification, header http.Header, body []byte) error
	// Acknowledge answers a handled notification the way the provider
	// expects (PayTR: the plain text "OK"), so it stops sending it again.
	Acknowledge(w http.ResponseWriter)
}

// Confirmer is implemented by providers whose result is read from their
// API with the page's token (iyzico Checkout Form): the browser return and
// the provider's webhook only say "ask now", and nothing in their body is
// trusted. Confirm answers the payment of our reference as the provider
// keeps it: StatusSucceeded, StatusFailed or StatusPending. It must check
// that the token belongs to orderRef. An error leaves the payment as it is.
type Confirmer interface {
	Confirm(ctx context.Context, account Account, orderRef, token string) (Notification, error)
}

// ConfirmerOf returns the confirmation call of a provider, when it has one.
func ConfirmerOf(p Provider) (Confirmer, bool) {
	c, ok := p.(Confirmer)
	return c, ok
}

// ChargeRequest charges a saved card without the payer on the page (a
// membership renewal, a no-show fee): the business needs the payer's
// consent to it beforehand.
type ChargeRequest struct {
	// OrderRef is our reference of the charge (letters and digits), unique
	// like a checkout's: an unknown result is looked up with it.
	OrderRef string
	Amount   int64
	Currency string
	ItemName string
	Buyer    Buyer
	Card     SavedCard
	Locale   string
	// Recurring marks a renewal of a membership.
	Recurring bool
}

// ChargeResult is a succeeded charge.
type ChargeResult struct {
	ProviderPaymentID string
	// PaidAmount is what was taken (installment interest included).
	PaidAmount int64
	Test       bool
}

// CardCharger is implemented by providers that charge a saved card through
// their API. The call classifies its failure like a refund: a
// *ProviderError with Temporary false is a definite refusal (nothing was
// taken); any other error means the result is unknown, and the charge is
// not sent again before CheckCharge (or a person) settled it.
type CardCharger interface {
	ChargeSavedCard(ctx context.Context, account Account, in ChargeRequest) (ChargeResult, error)
}

// CardChargerOf returns the saved card charge of a provider, when it has one.
func CardChargerOf(p Provider) (CardCharger, bool) {
	c, ok := p.(CardCharger)
	return c, ok
}

// ChargeChecker is implemented by providers that can tell what happened to
// a charge whose call had an unknown result, by our OrderRef (the request
// is the one the charge was sent with). It only asks.
// The answer's Status is StatusSucceeded, StatusFailed (the provider has no
// such payment or refused it) or StatusPending; an error leaves it unknown.
type ChargeChecker interface {
	CheckCharge(ctx context.Context, account Account, in ChargeRequest) (Notification, error)
}

// ChargeCheckerOf returns the charge status query of a provider, when it
// has one.
func ChargeCheckerOf(p Provider) (ChargeChecker, bool) {
	c, ok := p.(ChargeChecker)
	return c, ok
}

// CardRemover is implemented by providers that delete a saved card at the
// provider when the payer or the business removes it from Takwim.
type CardRemover interface {
	RemoveSavedCard(ctx context.Context, account Account, card SavedCard) error
}

// CardRemoverOf returns the card removal of a provider, when it has one.
func CardRemoverOf(p Provider) (CardRemover, bool) {
	c, ok := p.(CardRemover)
	return c, ok
}

// RefundRequest pays back (part of) a succeeded payment through the
// provider (TEC-913).
type RefundRequest struct {
	// OrderRef is our reference of the refunded payment (CheckoutRequest.OrderRef).
	OrderRef string
	// ProviderPaymentID is the provider's reference of the payment, when it
	// gave one.
	ProviderPaymentID string
	Amount            int64
	Currency          string
	// RefundRef is our reference of this refund (letters and digits): the
	// provider keeps it with the refund where it supports one.
	RefundRef string
}

// RefundResult is a refund the provider carried out.
type RefundResult struct {
	// ProviderRefundID is the provider's reference of the refund ("" when it
	// gives none).
	ProviderRefundID string
	Test             bool
}

// Refunder is implemented by providers that refund through their API. The
// call must classify its failure: a *ProviderError with Temporary false is a
// definite refusal (nothing was refunded); a Temporary error or any other
// error means the result is unknown, and the refund is not sent again
// before a person checked the provider's panel.
type Refunder interface {
	Refund(ctx context.Context, account Account, in RefundRequest) (RefundResult, error)
}

// RefunderOf returns the refund call of a provider, when it has one.
func RefunderOf(p Provider) (Refunder, bool) {
	r, ok := p.(Refunder)
	return r, ok
}

// What a refund status query found (TEC-978).
const (
	// RefundFound: the provider carried the refund out.
	RefundFound = "succeeded"
	// RefundNotDone: the provider has no such refund, or refused it:
	// nothing was paid back.
	RefundNotDone = "not_done"
	// RefundStillPending: the provider has the refund but not its final
	// state yet; ask again later.
	RefundStillPending = "pending"
)

// RefundCheck is the answer of a refund status query.
type RefundCheck struct {
	// Status is RefundFound, RefundNotDone or RefundStillPending.
	Status string
	// ProviderRefundID is the provider's reference of the refund, when found.
	ProviderRefundID string
	// Message is the provider's reason (a refusal's code and text).
	Message string
}

// RefundChecker is implemented by providers that can tell what happened to
// a refund whose call had an unknown result, by our RefundRef or the
// payment's references. It only asks; it never refunds. An error means the
// state is still unknown: the refund is asked about again later and never
// sent again.
type RefundChecker interface {
	CheckRefund(ctx context.Context, account Account, in RefundRequest) (RefundCheck, error)
}

// RefundCheckerOf returns the refund status query of a provider, when it
// has one.
func RefundCheckerOf(p Provider) (RefundChecker, bool) {
	c, ok := p.(RefundChecker)
	return c, ok
}

var (
	// ErrInvalidSignature: a notification did not verify.
	ErrInvalidSignature = errors.New("payments: invalid signature")
	// ErrMalformed: the provider message could not be read.
	ErrMalformed = errors.New("payments: malformed provider message")
)

// ProviderError is a provider refusal (code + message, never card data);
// Temporary marks network / 5xx failures worth trying again.
type ProviderError struct {
	Code      string
	Message   string
	Temporary bool
}

func (e *ProviderError) Error() string {
	if e.Code != "" {
		return "payments: provider error " + e.Code + ": " + e.Message
	}
	return "payments: provider error: " + e.Message
}

// Account carries decrypted credentials only for the duration of a provider call.
type Account struct {
	Mode        string
	UUID        string
	Credentials map[string]string
}

// Gateway retains Provider's hosted checkout and verification contract. Providers
// implement optional capabilities below; unsupported operations fail explicitly.
type Gateway = Provider

// PaymentGateway is the public name of the common gateway contract.
type PaymentGateway = Provider

// ThreeDSInitializer opens a provider-hosted 3DS flow; raw card data stays there.
type ThreeDSInitializer interface {
	InitThreeDS(context.Context, Account, CheckoutRequest) (Checkout, error)
}

// CardTokenizer starts hosted card registration without accepting PAN/CVV.
type CardTokenizer interface {
	TokenizeCard(context.Context, Account, CheckoutRequest) (Checkout, error)
}

// Webhook is the signed identity portion of an iyzico notification.
type Webhook struct {
	EventType      string
	PaymentID      string
	Token          string
	ConversationID string
	Status         string
}

type CardLister interface {
	ListSavedCards(context.Context, Account, string) ([]SavedCard, error)
}

// PaymentCanceller voids a captured payment when the provider allows it.
type PaymentCanceller interface {
	CancelPayment(context.Context, Account, RefundRequest) (RefundResult, error)
}
