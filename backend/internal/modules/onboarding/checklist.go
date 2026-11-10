package onboarding

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
)

// The set-up order (CF-198). The customer team's own write-up showed why:
// 37 articles were entered first and the customer was connected last, so
// the AI read articles without knowing who it was talking to. Every step
// has conditions the product can observe; the wizard and the assistant's
// "what did you leave out" both read this list.
const (
	// StepCustomer: introduce the customer — identity key, external
	// identity, package, company profile.
	StepCustomer = "customer"
	// StepSystem: connect the customer's own system (CF-197).
	StepSystem = "system"
	// StepAI: teach the AI — turn it on and publish a procedure (intent).
	StepAI = "ai"
	// StepKnowledge: fill the knowledge base.
	StepKnowledge = "knowledge"
	// StepAutomation: automation rules or workflows and an SLA policy.
	StepAutomation = "automation"
)

// StepOrder is the order the checklist shows and the assistant follows.
var StepOrder = []string{StepCustomer, StepSystem, StepAI, StepKnowledge, StepAutomation}

// Condition keys, per step.
const (
	CondIdentityKey      = "identity_key"
	CondExternalIdentity = "external_identity"
	CondPackage          = "package"
	CondCompany          = "company"
	CondSystem           = "system"
	CondAIEnabled        = "ai_enabled"
	CondProcedure        = "procedure"
	CondArticles         = "articles"
	CondSLAPolicy        = "sla_policy"
	CondAutomation       = "automation"
)

// StepConditions lists each step's condition keys in display order.
var StepConditions = map[string][]string{
	StepCustomer:   {CondIdentityKey, CondExternalIdentity, CondPackage, CondCompany},
	StepSystem:     {CondSystem},
	StepAI:         {CondAIEnabled, CondProcedure},
	StepKnowledge:  {CondArticles},
	StepAutomation: {CondSLAPolicy, CondAutomation},
}

// Facts are the observations the conditions are decided on.
type Facts struct {
	// IdentityKey: a web widget exists and has an identity key.
	IdentityKey bool
	// ExternalIdentities: contacts or companies carrying an external id
	// (CF-195).
	ExternalIdentities int64
	// PackagedCompanies: companies with a tier or a revenue plan (their
	// package).
	PackagedCompanies int64
	// CompanyProfile: the organization's legal name, website or phone is
	// filled in.
	CompanyProfile bool
	// Systems: connected customer systems (CF-197).
	Systems   int64
	AIEnabled bool
	// Procedures: published AI procedures (each one teaches an intent).
	Procedures  int64
	Articles    int64
	SLAPolicies int64
	// Automations: automation rules plus workflows.
	Automations int64
}

// FactsOf reads the facts from the counts row; IdentityKey stays false
// until the channels module answers.
func FactsOf(c db.OrganizationSetupCountsRow) Facts {
	return Facts{
		ExternalIdentities: c.ExternalContacts + c.ExternalCompanies, PackagedCompanies: c.PackagedCompanies,
		CompanyProfile: c.CompanyProfile, Systems: c.Systems, AIEnabled: c.AiEnabled, Procedures: c.Procedures,
		Articles: c.Articles, SLAPolicies: c.SlaPolicies, Automations: c.Automations,
	}
}

// holds decides one condition.
func (f Facts) holds(cond string) bool {
	switch cond {
	case CondIdentityKey:
		return f.IdentityKey
	case CondExternalIdentity:
		return f.ExternalIdentities > 0
	case CondPackage:
		return f.PackagedCompanies > 0
	case CondCompany:
		return f.CompanyProfile
	case CondSystem:
		return f.Systems > 0
	case CondAIEnabled:
		return f.AIEnabled
	case CondProcedure:
		return f.Procedures > 0
	case CondArticles:
		return f.Articles > 0
	case CondSLAPolicy:
		return f.SLAPolicies > 0
	case CondAutomation:
		return f.Automations > 0
	}
	return false
}

// Evaluate builds the steps in StepOrder: a step is done when every
// condition holds; a skipped step that is not done stays flagged.
func Evaluate(f Facts, skipped map[string]bool) []Step {
	out := make([]Step, 0, len(StepOrder))
	for _, key := range StepOrder {
		step := Step{Key: key, Done: true, Conditions: []Condition{}}
		for _, cond := range StepConditions[key] {
			ok := f.holds(cond)
			step.Done = step.Done && ok
			step.Conditions = append(step.Conditions, Condition{Key: cond, Done: ok})
		}
		step.Skipped = !step.Done && skipped[key]
		out = append(out, step)
	}
	return out
}

// Next is the first step that is neither done nor skipped ("" when none).
func Next(steps []Step) string {
	for _, s := range steps {
		if !s.Done && !s.Skipped {
			return s.Key
		}
	}
	return ""
}

// MissingKeys lists the unmet conditions of every step in order as
// "step.condition"; skipped steps are included (they are warnings).
func MissingKeys(steps []Step) []string {
	out := []string{}
	for _, s := range steps {
		for _, c := range s.Conditions {
			if !c.Done {
				out = append(out, s.Key+"."+c.Key)
			}
		}
	}
	return out
}
