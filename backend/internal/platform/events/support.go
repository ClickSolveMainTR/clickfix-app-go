package events

// Support analytics, survey and availability events (CF-99/102/103/110).
const (
	SurveyInvited               = "surveys.invited"
	SurveyResponded             = "surveys.responded"
	StatusChanged               = "status.changed"
	StatusNotifyContacts        = "status.notify_contacts"
	StatusSubscriptionRequested = "status.subscription_requested"
)

func supportCatalogEvents() []string {
	return []string{SurveyInvited, SurveyResponded, StatusChanged, StatusNotifyContacts, StatusSubscriptionRequested}
}
