package channels

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	platformchannels "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// Pages is the channels page state (CF-187, scenario 3): missing
// channels, channels in error, API channels that never received a message
// and the connection tests the assistant can run.
func Pages() []*capability.Page {
	return []*capability.Page{{
		Module: "channels", Routes: []string{"/channels"}, Permission: rbac.PermTenantChannelsRead,
		State: func(ctx context.Context, e capability.Exec) (capability.PageState, error) {
			st := capability.PageState{Module: "channels", Title: t(e, "Kanallar", "Channels")}
			items, err := listChannels(ctx, e.API, "")
			if err != nil {
				return st, err
			}
			if len(items) == 0 {
				st.Missing = append(st.Missing, capability.Step{Key: "connect_channel", Capability: "channels.connect",
					Title:  t(e, "Henüz bağlı kanal yok", "No channel connected yet"),
					Prompt: t(e, "Müşterilerimle konuşmak için ilk kanalımı bağlamama yardım et", "Help me connect my first channel")})
			}
			email := false
			for _, v := range items {
				email = email || v.Type == platformchannels.TypeEmail
			}
			if len(items) > 0 && !email {
				st.Missing = append(st.Missing, capability.Step{Key: "email_channel", Capability: "channels.connect",
					Title:  t(e, "Destek e-posta adresi bağlı değil", "No support e-mail address connected"),
					Prompt: t(e, "Destek e-posta adresimi bağla", "Connect my support e-mail address"),
					Focus:  &capability.Focus{Route: "/channels/connect/email"}})
			}
			for i, v := range items {
				route := "/channels/" + v.UUID.String()
				if v.Health.State == "error" {
					st.Warnings = append(st.Warnings, capability.Step{Key: "channel_error:" + v.UUID.String(), Capability: "channels.test",
						Title: t(e, "“%s” bağlantı hatası veriyor", "“%s” has a connection error", v.Name), Detail: clip(v.Health.Detail, 200),
						Prompt: t(e, "“%s” kanalını test et ve sorunu bul", "Test the “%s” channel and find the problem", v.Name),
						Focus:  &capability.Focus{Route: route}})
				}
				if v.Type == platformchannels.TypeAPI && v.LastInboundAt == nil {
					st.Warnings = append(st.Warnings, capability.Step{Key: "api_silent:" + v.UUID.String(), Capability: "channels.api.sample_request",
						Title:  t(e, "“%s” henüz mesaj almadı", "“%s” has not received a message yet", v.Name),
						Prompt: t(e, "“%s” kanalı için örnek istek ver", "Give me an example request for the “%s” channel", v.Name)})
				}
				if i < 3 {
					st.Tests = append(st.Tests, capability.Test{Capability: "channels.test",
						Title:  t(e, "“%s” kanalını test et", "Test “%s”", v.Name),
						Prompt: t(e, "“%s” kanalını test et", "Test the “%s” channel", v.Name)})
				}
			}
			return st, nil
		},
	}}
}
