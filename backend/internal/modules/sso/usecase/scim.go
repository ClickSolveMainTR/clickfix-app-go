package usecase

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/password"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const UserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"
const GroupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"

type SCIMUser struct {
	Schemas    []string  `json:"schemas"`
	ID         uuid.UUID `json:"id"`
	ExternalID string    `json:"externalId"`
	UserName   string    `json:"userName"`
	Active     *bool     `json:"active"`
	Name       struct {
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
}

func UserView(id uuid.UUID, external, email, name, surname string, active bool) SCIMUser {
	v := SCIMUser{Schemas: []string{UserSchema}, ID: id, ExternalID: external, UserName: email, Active: &active}
	v.Name.GivenName = name
	v.Name.FamilyName = surname
	return v
}
func (s *Service) Provision(ctx context.Context, org int64, in SCIMUser) (SCIMUser, error) {
	addr, e := mail.ParseAddress(in.UserName)
	if e != nil || addr.Address != in.UserName || len(in.ExternalID) == 0 || len(in.ExternalID) > 500 || len(in.Name.GivenName) > 100 || len(in.Name.FamilyName) > 100 {
		return SCIMUser{}, ErrInvalid
	}
	if in.Active != nil && !*in.Active {
		return SCIMUser{}, ErrInvalid
	}
	var result SCIMUser
	e = s.Repo.Transaction(ctx, func(q *db.Queries) error {
		id, e := q.SCIMFindUser(ctx, strings.ToLower(in.UserName))
		if errors.Is(e, pgx.ErrNoRows) {
			hash, e := password.HashUnusable()
			if e != nil {
				return e
			}
			id, e = q.SCIMCreateUser(ctx, db.SCIMCreateUserParams{Email: strings.ToLower(in.UserName), PasswordHash: hash, Name: in.Name.GivenName, Surname: in.Name.FamilyName})
			if e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		if e = q.SCIMMember(ctx, db.SCIMMemberParams{OrganizationID: org, UserID: id}); e != nil {
			return e
		}
		row, e := q.SCIMIdentity(ctx, db.SCIMIdentityParams{OrganizationID: org, UserID: id, ExternalID: in.ExternalID})
		if e != nil {
			return e
		}
		// Profile belongs to the global account; only membership and IdP link are org-owned.
		user, e := q.GetUserByID(ctx, id)
		if e != nil {
			return e
		}
		result = UserView(row.Uuid, row.ExternalID, user.Email, user.Name, user.Surname, true)
		return nil
	})
	return result, e
}
func (s *Service) SetActive(ctx context.Context, org int64, id uuid.UUID, active bool) (SCIMUser, error) {
	var result SCIMUser
	err := s.Repo.Transaction(ctx, func(q *db.Queries) error {
		row, e := q.SCIMUser(ctx, db.SCIMUserParams{OrganizationID: org, Uuid: id})
		if e != nil {
			return e
		}
		protected, e := q.SCIMProtectedMember(ctx, db.SCIMProtectedMemberParams{OrganizationID: org, UserID: row.UserID})
		if e != nil {
			return e
		}
		if protected && !active {
			return ErrDenied
		}
		if e = q.SCIMSetActive(ctx, db.SCIMSetActiveParams{OrganizationID: org, Uuid: id, Active: active}); e != nil {
			return e
		}
		if active {
			e = q.SCIMMember(ctx, db.SCIMMemberParams{OrganizationID: org, UserID: row.UserID})
		} else {
			e = q.SCIMRemoveMember(ctx, db.SCIMRemoveMemberParams{OrganizationID: org, UserID: row.UserID})
		}
		if e != nil {
			return e
		}
		result = UserView(row.Uuid, row.ExternalID, row.Email, row.Name, row.Surname, active)
		return nil
	})
	return result, err
}

type Member struct {
	Value   uuid.UUID `json:"value"`
	Display string    `json:"display,omitempty"`
}
type SCIMGroup struct {
	Schemas     []string  `json:"schemas"`
	ID          uuid.UUID `json:"id"`
	ExternalID  string    `json:"externalId"`
	DisplayName string    `json:"displayName"`
	Members     []Member  `json:"members"`
}

func (s *Service) Group(ctx context.Context, org int64, id uuid.UUID) (SCIMGroup, error) {
	row, e := s.Repo.Q.SCIMGroup(ctx, db.SCIMGroupParams{OrganizationID: org, Uuid: id})
	if e != nil {
		return SCIMGroup{}, e
	}
	return group(ctx, s.Repo.Q, org, row.Uuid, row.ExternalID, row.Name, row.TeamID)
}
func group(ctx context.Context, q *db.Queries, org int64, id uuid.UUID, external, name string, team int64) (SCIMGroup, error) {
	rows, e := q.SCIMGroupMembers(ctx, db.SCIMGroupMembersParams{OrganizationID: org, TeamID: team})
	if e != nil {
		return SCIMGroup{}, e
	}
	v := SCIMGroup{Schemas: []string{GroupSchema}, ID: id, ExternalID: external, DisplayName: name, Members: []Member{}}
	for _, m := range rows {
		v.Members = append(v.Members, Member{m.Uuid, m.Name})
	}
	return v, nil
}
func (s *Service) SaveGroup(ctx context.Context, org int64, id uuid.UUID, in SCIMGroup) (SCIMGroup, error) {
	if len(strings.TrimSpace(in.DisplayName)) == 0 || len(in.DisplayName) > 120 || len(in.ExternalID) > 500 || len(in.Members) > 1000 {
		return SCIMGroup{}, ErrInvalid
	}
	var out SCIMGroup
	e := s.Repo.Transaction(ctx, func(q *db.Queries) error {
		team := int64(0)
		if id == uuid.Nil {
			if in.ExternalID == "" {
				return ErrInvalid
			}
			n, e := q.SCIMCreateTeam(ctx, db.SCIMCreateTeamParams{OrganizationID: org, Name: in.DisplayName})
			if e != nil {
				return e
			}
			team = n
			g, e := q.SCIMCreateGroup(ctx, db.SCIMCreateGroupParams{OrganizationID: org, TeamID: team, ExternalID: in.ExternalID})
			if e != nil {
				return e
			}
			id = g.Uuid
		} else {
			g, e := q.SCIMGroup(ctx, db.SCIMGroupParams{OrganizationID: org, Uuid: id})
			if e != nil {
				return e
			}
			if in.ExternalID != "" && in.ExternalID != g.ExternalID {
				return ErrInvalid
			}
			in.ExternalID = g.ExternalID
			team = g.TeamID
			if e = q.SCIMRenameTeam(ctx, db.SCIMRenameTeamParams{OrganizationID: org, ID: team, Name: in.DisplayName}); e != nil {
				return e
			}
		}
		if e := q.SCIMClearMembers(ctx, db.SCIMClearMembersParams{OrganizationID: org, TeamID: team}); e != nil {
			return e
		}
		seen := map[uuid.UUID]bool{}
		for _, m := range in.Members {
			if seen[m.Value] {
				continue
			}
			seen[m.Value] = true
			n, e := q.SCIMAddMember(ctx, db.SCIMAddMemberParams{OrganizationID: org, TeamID: team, Uuid: m.Value})
			if e != nil {
				return e
			}
			if n != 1 {
				return ErrInvalid
			}
		}
		var e error
		out, e = group(ctx, q, org, id, in.ExternalID, in.DisplayName, team)
		return e
	})
	return out, e
}
