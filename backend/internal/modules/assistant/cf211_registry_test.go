package assistant_test

import (
	aiagentsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents"
	appointmentsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/appointments"
	auditmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/audit"
	billingmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing"
	contactsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts"
	conversationsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	customfieldsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields"
	knowledgemodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge"
	macrosmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/macros"
	organizationsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/organizations"
	reportsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/reports"
	statusmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/statuspage"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

// budgetExtraCapabilities are the capabilities registered by CF-211, in
// the order of httpserver/server.go.
func budgetExtraCapabilities(*fixture) [][]*capability.Capability { return cf211Groups() }

// cf211Capabilities are the capabilities CF-211 added or opened to the
// assistant: the inbox ones and the new modules.
func cf211Capabilities() []*capability.Capability {
	out := conversationsmodule.Capabilities()
	for _, g := range cf211Groups() {
		out = append(out, g...)
	}
	return out
}

func cf211Groups() [][]*capability.Capability {
	return [][]*capability.Capability{
		contactsmodule.Capabilities(), macrosmodule.Capabilities(), knowledgemodule.Capabilities(),
		appointmentsmodule.Capabilities(), reportsmodule.Capabilities(), billingmodule.Capabilities(),
		organizationsmodule.Capabilities(), auditmodule.Capabilities(), customfieldsmodule.Capabilities(),
		statusmodule.Capabilities(), aiagentsmodule.Capabilities(),
	}
}
