package billingtest_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/billingtest"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/model"
)

func TestIsCatalogConflict(t *testing.T) {
	cases := map[error]bool{
		fmt.Errorf("%w: missing feature module.sim_1", model.ErrInvalid):           true,
		fmt.Errorf("%w: unknown/duplicate feature module.sim_1", model.ErrInvalid): true,
		fmt.Errorf("%w: price", model.ErrInvalid):                                  false,
		errors.New("missing feature without the sentinel"):                         false,
		fmt.Errorf("grant insert: %w", model.ErrConflict):                          true,
		model.ErrNotFound: false,
	}
	for err, want := range cases {
		if got := billingtest.IsCatalogConflict(err); got != want {
			t.Errorf("%v: got %v want %v", err, got, want)
		}
	}
}
