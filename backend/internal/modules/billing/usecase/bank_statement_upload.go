package usecase

import (
	"fmt"
	"io"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/iban"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
)

// ParseBankStatement uses the shared ioengine file decoders. A dedicated
// financial import bypasses ioengine rollback: settled payments cannot be
// undone by deleting an imported row. Corrections use the billing refund flow.
// Required columns: external_id, booked_at, amount, currency. Optional columns:
// description, reference, sender_iban. Only incoming credits are accepted.
func ParseBankStatement(format string, source io.Reader) ([]BankStatementInput, error) {
	if format == "mt940" {
		return parseMT940(source)
	}
	if format != "csv" && format != "xlsx" {
		return nil, model.ErrInvalid
	}
	_, rows, err := ioengine.ParseUpload(ioengine.ImportFormat(format), source)
	if err != nil {
		return nil, fmt.Errorf("%w: statement file: %s", model.ErrInvalid, err)
	}
	if len(rows) == 0 || len(rows) > 1000 {
		return nil, model.ErrInvalid
	}
	out := make([]BankStatementInput, 0, len(rows))
	for index, raw := range rows {
		row := map[string]string{}
		for key, value := range raw {
			row[strings.TrimSpace(strings.TrimPrefix(key, "\ufeff"))] = strings.TrimSpace(value)
		}
		amount, err := parseSheetAmount(row["amount"])
		if err != nil {
			return nil, fmt.Errorf("%w: row %d amount", model.ErrInvalid, index+1)
		}
		date, err := parseSheetDate(row["booked_at"])
		if err != nil {
			return nil, fmt.Errorf("%w: row %d booked_at", model.ErrInvalid, index+1)
		}
		in := BankStatementInput{ExternalID: row["external_id"], Description: row["description"], Reference: row["reference"], SenderIBAN: iban.Normalize(row["sender_iban"]), AmountMinor: amount.Minor(), Currency: row["currency"], BookedAt: date}
		if !in.valid() {
			return nil, fmt.Errorf("%w: row %d", model.ErrInvalid, index+1)
		}
		out = append(out, in)
	}
	return out, nil
}
