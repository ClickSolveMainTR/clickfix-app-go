package onboarding

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
)

// Copy of the steps and conditions for the assistant (tr / en).
var stepTitles = map[string][2]string{
	StepCustomer:   {"Müşteriyi tanıt", "Introduce the customer"},
	StepSystem:     {"Sistemini bağla", "Connect your system"},
	StepAI:         {"AI'a öğret", "Teach the AI"},
	StepKnowledge:  {"Bilgi bankası", "Knowledge base"},
	StepAutomation: {"Otomasyon ve SLA", "Automation and SLA"},
}

var conditionCopy = map[string][2]string{
	CondIdentityKey:      {"Widget kimlik anahtarı yok", "The widget has no identity key"},
	CondExternalIdentity: {"Dış kimlikli müşteri yok", "No customer carries an external id"},
	CondPackage:          {"Hiçbir şirketin paketi (seviye veya plan) yok", "No company has a package (tier or plan)"},
	CondCompany:          {"Şirket bilgileri boş", "The company profile is empty"},
	CondSystem:           {"Müşterinin sistemi bağlı değil", "The customer's system is not connected"},
	CondAIEnabled:        {"Yapay zekâ kapalı", "AI is off"},
	CondProcedure:        {"Yayınlanmış prosedür (niyet) yok", "No published procedure (intent)"},
	CondArticles:         {"Bilgi bankası boş", "The knowledge base is empty"},
	CondSLAPolicy:        {"SLA politikası yok", "No SLA policy"},
	CondAutomation:       {"Otomasyon kuralı veya iş akışı yok", "No automation rule or workflow"},
}

var conditionPrompts = map[string][2]string{
	CondIdentityKey:      {"Widget kimlik anahtarını nasıl tanımlarım?", "How do I set the widget identity key?"},
	CondExternalIdentity: {"Müşterilerime dış kimlik nasıl veririm?", "How do I give my customers an external id?"},
	CondPackage:          {"Şirketlere paket nasıl atarım?", "How do I assign packages to companies?"},
	CondCompany:          {"Şirket bilgilerini nereden doldururum?", "Where do I fill in the company profile?"},
	CondSystem:           {"Sistemimi ClickFix'e nasıl bağlarım?", "How do I connect my system to ClickFix?"},
	CondAIEnabled:        {"Yapay zekâ özelliklerini aç", "Turn on the AI features"},
	CondProcedure:        {"İlk prosedürü nasıl yazarım?", "How do I write the first procedure?"},
	CondArticles:         {"Bilgi bankasına hangi makalelerle başlamalıyım?", "Which articles should I start the knowledge base with?"},
	CondSLAPolicy:        {"SLA politikası oluşturmama yardım et", "Help me create an SLA policy"},
	CondAutomation:       {"İlk otomasyon kuralımı kurmama yardım et", "Help me set up my first automation rule"},
}

// conditionCapabilities names the assistant capability that does a condition
// (when one exists).
var conditionCapabilities = map[string]string{
	CondAIEnabled: "ai.enable", CondSLAPolicy: "sla.policies.create", CondAutomation: "workflows.create",
}

// Title is a step's title in the locale.
func Title(step string, en bool) string { return pick(stepTitles[step], en) }

// ConditionTitle is a condition's title in the locale.
func ConditionTitle(cond string, en bool) string { return pick(conditionCopy[cond], en) }

func pick(v [2]string, en bool) string {
	if en {
		return v[1]
	}
	return v[0]
}

// PageSteps turns the checklist into the assistant's page steps: unmet
// conditions of pending steps are "missing", those of skipped steps are
// "warnings", both in the set-up order.
func PageSteps(steps []Step, en bool) (missing, warnings []capability.Step) {
	missing, warnings = []capability.Step{}, []capability.Step{}
	for _, s := range steps {
		for _, c := range s.Conditions {
			if c.Done {
				continue
			}
			st := capability.Step{
				Key: s.Key + "." + c.Key, Title: ConditionTitle(c.Key, en), Detail: Title(s.Key, en),
				Capability: conditionCapabilities[c.Key], Prompt: pick(conditionPrompts[c.Key], en),
			}
			if s.Skipped {
				warnings = append(warnings, st)
			} else {
				missing = append(missing, st)
			}
		}
	}
	return missing, warnings
}

// Pages is the assistant page state of the set-up pages (CF-198): the
// home, the dashboard and the wizard show what is left in the set-up
// order.
func Pages(svc *Service) []*capability.Page {
	return []*capability.Page{{
		Module: "setup", Routes: []string{"/", "/dashboard", "/setup", "/getting-started"},
		State: func(ctx context.Context, e capability.Exec) (capability.PageState, error) {
			en := e.En()
			st := capability.PageState{Module: "setup", Title: pick([2]string{"Kurulum", "Set-up"}, en)}
			cl, err := svc.Checklist(ctx, e.OrgID)
			if err != nil {
				return st, err
			}
			st.Missing, st.Warnings = PageSteps(cl.Steps, en)
			return st, nil
		},
	}}
}
