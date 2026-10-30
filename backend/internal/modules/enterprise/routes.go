package enterprise

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/google/uuid"
	"net"
	"net/http"
)

func RegisterRoutes(rt supportops.Router, s *Service) {
	rt.Platform("GET /v1/platform/organizations/{organization}/quotes/meta", "platform.quotes.read", func(r *http.Request) (any, error) { return supportops.Meta("platform.quotes"), nil })
	org := func(r *http.Request) (int64, error) {
		id, _ := uuid.Parse(r.PathValue("organization"))
		return s.Repo.Q.BillingGetOrganizationID(r.Context(), id)
	}
	rt.Platform("POST /v1/platform/enterprise-quotes", "platform.quotes.write", func(r *http.Request) (any, error) {
		in, err := supportops.Read[Input](r)
		if err != nil {
			return nil, err
		}
		out, err := s.Create(r.Context(), supportops.Actor(r.Context()), in)
		return PrivateView(out), err
	})
	rt.Platform("GET /v1/platform/organizations/{organization}/quotes", "platform.quotes.read", func(r *http.Request) (any, error) {
		o, err := org(r)
		if err != nil {
			return nil, err
		}
		q, err := supportops.List(r)
		if err != nil {
			return nil, err
		}
		rows, err := s.Repo.Q.QuoteList(r.Context(), db.QuoteListParams{OrganizationID: o, RowLimit: q.Limit, RowOffset: q.Offset})
		if err != nil {
			return nil, err
		}
		total, err := s.Repo.Q.QuoteCount(r.Context(), o)
		return supportops.Page(rows, total, q), err
	})
	rt.Platform("GET /v1/platform/organizations/{organization}/quotes/{uuid}", "platform.quotes.read", func(r *http.Request) (any, error) {
		o, err := org(r)
		if err != nil {
			return nil, err
		}
		out, err := s.Repo.Q.QuoteLock(r.Context(), db.QuoteLockParams{OrganizationID: o, Uuid: supportops.ID(r)})
		return PrivateView(out), err
	})
	rt.Platform("POST /v1/platform/organizations/{organization}/quotes/{uuid}/send", "platform.quotes.write", func(r *http.Request) (any, error) {
		o, err := org(r)
		if err != nil {
			return nil, err
		}
		out, err := s.Repo.Q.QuoteSend(r.Context(), db.QuoteSendParams{OrganizationID: o, Uuid: supportops.ID(r)})
		return PrivateView(out), err
	})
	rt.Platform("POST /v1/platform/organizations/{organization}/quotes/{uuid}/convert", "platform.quotes.write", func(r *http.Request) (any, error) {
		o, err := org(r)
		if err != nil {
			return nil, err
		}
		out, err := s.Convert(r.Context(), o, supportops.Actor(r.Context()), supportops.ID(r))
		return PrivateView(out), err
	})
	rt.Mux.HandleFunc("GET /v1/public/enterprise-quotes/{uuid}", rt.Handler(func(r *http.Request) (any, error) {
		q, err := s.Repo.Q.QuotePublic(r.Context(), supportops.ID(r))
		if err != nil {
			return nil, err
		}
		return PublicView(q), nil
	}, false))
	rt.Mux.HandleFunc("POST /v1/public/enterprise-quotes/{uuid}/decision", rt.Handler(func(r *http.Request) (any, error) {
		in, err := supportops.Read[struct {
			Accept *bool `json:"accept"`
		}](r)
		if err != nil || in.Accept == nil {
			return nil, supportops.ErrInvalid
		}
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		q, err := s.Decide(r.Context(), supportops.ID(r), *in.Accept, ip)
		if err != nil {
			return nil, err
		}
		return PublicView(q), nil
	}, false))
	rt.Mux.HandleFunc("GET /v1/public/enterprise-quotes/{uuid}/pdf", func(w http.ResponseWriter, r *http.Request) {
		q, err := s.Repo.Q.QuotePublic(r.Context(), supportops.ID(r))
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		b, err := PDF(q)
		if err != nil {
			supportops.Fail(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", "attachment; filename=quote.pdf")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	})
}
