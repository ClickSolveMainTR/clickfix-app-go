package usecase

import (
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
)

func TestBankMatchEvidenceAndAmbiguity(t *testing.T) {
	row := db.BillingBankStatementLine{AmountMinor: 10000, Reference: "INV-123", SenderIban: "TR330006100519786457841326"}
	c := db.BillingBankStatementCandidatesRow{Uuid: uuid.New(), AmountMinor: 10000, PaymentReference: "INV-123"}
	for _, tc := range []struct {
		name      string
		reference string
		amount    int64
		sender    string
		duplicate bool
		exact     bool
	}{
		{name: "exact", reference: "INV-123", amount: 10000, exact: true},
		{name: "amount alone", reference: "unrelated", amount: 10000},
		{name: "substring collision", reference: "INV-1234", amount: 10000},
		{name: "wrong amount", reference: "INV-123", amount: 9999},
		{name: "IBAN conflict", reference: "INV-123", amount: 10000, sender: "TRdifferent"},
		{name: "ambiguous reference", reference: "INV-123", amount: 10000, duplicate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := row
			line.Reference = tc.reference
			line.AmountMinor = tc.amount
			candidate := c
			candidate.SenderIban = tc.sender
			candidates := []db.BillingBankStatementCandidatesRow{candidate}
			if tc.duplicate {
				candidates = append(candidates, candidate)
			}
			_, _, exact := scoreBankCandidates(line, candidates)
			if exact != tc.exact {
				t.Fatalf("exact=%v want %v", exact, tc.exact)
			}
		})
	}
}

func TestStatementCSVBankFormats(t *testing.T) {
	for _, amount := range []string{"1.250,50", "1,250.50"} {
		raw := "external_id,booked_at,amount,currency,description\ne1,28.10.2026,\"" + amount + "\",TRY,invoice\n"
		rows, err := ParseBankStatement("csv", strings.NewReader(raw))
		if err != nil || len(rows) != 1 || rows[0].AmountMinor != 125050 {
			t.Fatalf("rows=%+v err=%v", rows, err)
		}
	}
	for _, raw := range []string{
		"external_id,booked_at,amount,currency\ne1,2026-10-28,-100,TRY\n",
		"external_id,booked_at,amount,currency\ne1,2026-10-28,100,USD\n",
		"external_id,booked_at,amount,currency\n,2026-10-28,100,TRY\n",
		"external_id,booked_at,amount,currency\ne1,invalid,100,TRY\n",
	} {
		if _, err := ParseBankStatement("csv", strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestBankStatementFutureDateRejected(t *testing.T) {
	in := BankStatementInput{ExternalID: "future", Currency: "TRY", AmountMinor: 100, BookedAt: time.Now().Add(48 * time.Hour)}
	if in.valid() {
		t.Fatal("future credit accepted")
	}
}

func TestMT940CreditsOnlyAndAccountValidation(t *testing.T) {
	raw := ":20:statement\n:25:TR330006100519786457841326\n:60F:C261028TRY0,00\n:61:2610281028C100,00NTRFINV-123//BANK-1\n:86:EFT INV-123\ncontinued detail\n:61:2610281028D50,00NTRFNONREF//BANK-2\n:86:Debit must not import\n:62F:C261028TRY50,00\n"
	rows, err := ParseBankStatement("mt940", strings.NewReader(raw))
	if err != nil || len(rows) != 1 || rows[0].AmountMinor != 10000 || rows[0].ExternalID != "BANK-1" || !strings.Contains(rows[0].Description, "continued detail") {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	for _, bad := range []string{strings.Replace(raw, "TRY", "USD", 1), strings.Replace(raw, "TR330006100519786457841326", "TR000006100519786457841326", 1), strings.Replace(raw, "INV-123//BANK-1", "NONREF", 1)} {
		if _, err = ParseBankStatement("mt940", strings.NewReader(bad)); err == nil {
			t.Fatal("unsafe MT940 accepted")
		}
	}
}
